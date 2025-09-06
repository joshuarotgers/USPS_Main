(() => {
  const $ = (id) => document.getElementById(id);
  const headers = () => ({
    'Content-Type': 'application/json',
    'X-Tenant-Id': $('tenant').value.trim() || 't_demo',
    'X-Role': 'admin'
  });

  const map = L.map('map').setView([37.7749, -122.4194], 12);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '&copy; OpenStreetMap contributors'
  }).addTo(map);

  const geofenceLayer = L.layerGroup().addTo(map);
  const routeLineLayer = L.layerGroup().addTo(map);
  const driverLayer = L.layerGroup().addTo(map);
  const groupLayer = L.layerGroup().addTo(map);
  let addMode = false;
  let sse;
  let driverMarkers = {};
  let knownDrivers = new Set();
  let routeDrivers = new Set();
  let currentRouteId = null;
  let currentPath = [];
  let simTimer = null;
  let simIndex = 0;
  let simForward = true;
  let pathCum = [];
  let pathTotal = 0;
  let simDrivers = [];
  let groupDebounce = null;
  let heatDebounce = null;

  const log = (obj) => {
    const pre = $('events');
    const line = typeof obj === 'string' ? obj : JSON.stringify(obj);
    pre.textContent += `\n${new Date().toISOString()} ${line}`;
    pre.scrollTop = pre.scrollHeight;
  };
  const driverLast = new Map();

  async function fetchJSON(url, opts) {
    const res = await fetch(url, opts);
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return await res.json();
  }

  function drawGeofences(items) {
    geofenceLayer.clearLayers();
    (items || []).forEach((g) => {
      const c = g.center || { lat: g.lat, lng: g.lng };
      if (!c || typeof c.lat !== 'number' || typeof c.lng !== 'number') return;
      const radius = g.radiusM || g.radius_m || 200;
      const circle = L.circle([c.lat, c.lng], { radius, color: '#3366ff', fillOpacity: 0.1 });
      const name = g.name || g.id;
      circle.bindPopup(() => {
        const div = document.createElement('div');
        div.innerHTML = `<div style="margin-bottom:6px;"><strong>${name}</strong><br/><small>${g.id}</small><br/>Radius: ${radius} m</div>`;
        const btn = document.createElement('button');
        btn.textContent = 'Delete';
        btn.style.padding = '6px 10px';
        btn.onclick = async () => {
          try {
            await fetch(`/v1/geofences/${g.id}`, { method: 'DELETE', headers: headers() });
            await refreshGeofences();
            circle.closePopup();
          } catch (e) { alert(`Delete failed: ${e.message}`); }
        };
        div.appendChild(btn);
        return div;
      });
      circle.addTo(geofenceLayer);
    });
  }

  async function refreshGeofences() {
    try {
      const data = await fetchJSON('/v1/geofences', { headers: headers() });
      drawGeofences(data.items || data || []);
    } catch (e) {
      console.error(e); alert('Failed to load geofences');
    }
  }

  async function createGeofence(lat, lng) {
    const radius = parseInt(($('radius').value || '200'), 10) || 200;
    const name = `GF ${lat.toFixed(4)},${lng.toFixed(4)}`;
    const body = {
      name,
      type: 'circle',
      radiusM: radius,
      center: { lat, lng }
    };
    const res = await fetch('/v1/geofences', { method: 'POST', headers: headers(), body: JSON.stringify(body) });
    if (!res.ok) { const t = await res.text(); throw new Error(`${res.status} ${t}`); }
    await refreshGeofences();
  }

  function setAddMode(on) {
    addMode = on;
    $('btn-add').classList.toggle('active', !!on);
    if (on) {
      map.getContainer().style.cursor = 'crosshair';
    } else {
      map.getContainer().style.cursor = '';
    }
  }

  map.on('click', async (e) => {
    if (!addMode) return;
    try {
      await createGeofence(e.latlng.lat, e.latlng.lng);
    } catch (err) {
      alert(`Create failed: ${err.message}`);
    } finally {
      setAddMode(false);
    }
  });

  async function listRoutes() {
    try {
      const data = await fetchJSON('/v1/routes?limit=50', { headers: headers() });
      const items = data.items || [];
      const holder = $('routes');
      holder.innerHTML = '';
      if (items.length === 0) {
        holder.textContent = 'No routes';
        return;
      }
      items.forEach((r) => {
        const div = document.createElement('div');
        div.className = 'route-item';
        const id = document.createElement('span');
        id.textContent = r.id;
        id.style.fontFamily = 'monospace';
        id.style.fontSize = '12px';
        const btn = document.createElement('button');
        btn.textContent = 'Subscribe';
        btn.onclick = () => subscribeRoute(r.id);
        div.appendChild(id); div.appendChild(btn);
        holder.appendChild(div);
      });
    } catch (e) { console.error(e); alert('Failed to list routes'); }
  }

  function closeSSE() { if (sse) { sse.close(); sse = null; } }

  function buildPathCache(pts) {
    pathCum = [0];
    pathTotal = 0;
    for (let i = 1; i < pts.length; i++) {
      const a = pts[i-1], b = pts[i];
      const d = haversineM(a.lat, a.lng, b.lat, b.lng);
      pathTotal += d;
      pathCum.push(pathTotal);
    }
  }

  async function drawRoute(routeId) {
    routeLineLayer.clearLayers();
    driverLayer.clearLayers();
    driverMarkers = {};
    try {
      const data = await fetchJSON(`/v1/routes/${routeId}/path`, { headers: headers() });
      const pts = (data && data.points) || [];
      currentPath = pts;
      buildPathCache(pts);
      if (pts.length > 0) {
        const latlngs = pts.map(p => [p.lat, p.lng]);
        const poly = L.polyline(latlngs, { color: '#ff6b6b', weight: 4 });
        poly.addTo(routeLineLayer);
        map.fitBounds(poly.getBounds(), { padding: [40, 40] });
      }
    } catch (e) { /* silent */ }
  }

  function subscribeRoute(routeId) {
    closeSSE();
    log(`Subscribing route ${routeId}`);
    currentRouteId = routeId;
    stopSimulation();
    drawRoute(routeId);
    // Load latest driver locations for this route
    (async () => {
      try {
        const data = await fetchJSON(`/v1/routes/${routeId}/drivers/latest`, { headers: headers() });
        const items = (data && data.items) || [];
        routeDrivers = new Set();
        for (const it of items) { upsertDriverMarker(it.driverId, it.lat, it.lng); addKnownDriver(it.driverId); routeDrivers.add(it.driverId); driverLast.set(it.driverId, Date.now()); }
        refreshFollowList();
        if ($('only-route').checked) applyRouteFilter();
      } catch (e) { /* ignore */ }
    })();
    const hdrs = headers();
    const params = new URLSearchParams();
    Object.entries(hdrs).forEach(([k,v]) => params.append(k, v));
    const url = `/v1/routes/${routeId}/events/stream?${params.toString()}`;
    sse = new EventSource(url);
    sse.onmessage = (ev) => log({ event: 'message', data: ev.data });
    sse.addEventListener('heartbeat', (ev) => log({ event: 'heartbeat', data: ev.data }));
    sse.addEventListener('stop.advanced', (ev) => log({ event: 'stop.advanced', data: ev.data }));
    sse.addEventListener('driver.location', (ev) => {
      try {
        const d = JSON.parse(ev.data);
        const id = d.driverId || 'driver';
        upsertDriverMarker(id, d.lat, d.lng);
        addKnownDriver(id); routeDrivers.add(id); refreshFollowList();
        driverLast.set(id, Date.now());
        if ($('follow').checked) {
          const sel = $('follow-driver').value || '__ANY__';
          if (sel === '__ANY__' || sel === id) { map.panTo([d.lat, d.lng], { animate: true }); }
        }
        if ($('group-toggle').checked) scheduleGroupRecompute();
        if ($('only-route').checked) applyRouteFilter();
      } catch {}
    });
    sse.onerror = (err) => { log('SSE error'); };
  }

  // Deterministic color palette
  const palette = [
    '#e6194B','#3cb44b','#0082c8','#f58231','#911eb4','#46f0f0','#f032e6','#d2f53c','#fabebe','#008080',
    '#e6beff','#aa6e28','#fffac8','#800000','#aaffc3','#808000','#ffd8b1','#000080','#808080','#000000'
  ];
  function colorForId(id){
    let h=0; for (let i=0;i<id.length;i++){ h=(h*33 + id.charCodeAt(i))>>>0; }
    return palette[h % palette.length];
  }
  function upsertDriverMarker(id, lat, lng){
    const color = colorForId(id||'driver');
    const sel = $('follow-driver') ? $('follow-driver').value : '__ANY__';
    const selected = (sel !== '__ANY__' && sel === id);
    const pinCls = selected ? 'driver-pin sel' : 'driver-pin';
    const lblCls = selected ? 'driver-label sel' : 'driver-label';
    const html = `<div class="driver-wrap"><i class="${pinCls}" style="background:${color}"></i><span class="${lblCls}">${id}</span></div>`;
    const icon = L.divIcon({ className: '', html, iconSize: [1,1], iconAnchor: [0,0] });
    const ll = [lat, lng];
    const shouldShow = (!$('only-route').checked) || routeDrivers.has(id);
    if (!driverMarkers[id]) {
      driverMarkers[id] = L.marker(ll, { title: id, icon });
    }
    driverMarkers[id].setLatLng(ll); driverMarkers[id].setIcon(icon);
    // toggle visibility
    const inLayer = driverLayer.hasLayer(driverMarkers[id]);
    if (shouldShow && !inLayer) driverLayer.addLayer(driverMarkers[id]);
    if (!shouldShow && inLayer) driverLayer.removeLayer(driverMarkers[id]);
  }

  function scheduleGroupRecompute(){ if (groupDebounce) clearTimeout(groupDebounce); const n=Object.keys(driverMarkers).length; const delay = n>200?700:350; groupDebounce = setTimeout(recomputeGroups, delay); }
  function recomputeGroups(){
    groupLayer.clearLayers();
    const on = $('group-toggle').checked;
    // show/hide individual markers
    Object.values(driverMarkers).forEach(m=>{ if (on) { driverLayer.removeLayer(m); } else { driverLayer.addLayer(m); }});
    if (!on) return;
    // simple grid cluster based on zoom
    const zoom = map.getZoom();
    const cellDeg = Math.max(0.002, 0.5 / Math.pow(2, zoom));
    const buckets = new Map();
    for (const [id, m] of Object.entries(driverMarkers)){
      const p = m.getLatLng();
      const gx = Math.floor(p.lat / cellDeg);
      const gy = Math.floor(p.lng / cellDeg);
      const key = gx+','+gy;
      if (!buckets.has(key)) buckets.set(key, {count:0, lat:0, lng:0});
      const b = buckets.get(key); b.count += 1; b.lat += p.lat; b.lng += p.lng;
    }
    for (const b of buckets.values()){
      const lat = b.lat / b.count, lng = b.lng / b.count;
      const icon = L.divIcon({ className: '', html: `<div class="grp">${b.count}</div>`, iconSize:[26,26], iconAnchor:[13,13] });
      L.marker([lat,lng], { icon }).addTo(groupLayer);
    }
  }

  map.on('zoomend moveend', ()=>{ if ($('group-toggle').checked) scheduleGroupRecompute(); if ($('heat-toggle').checked) scheduleHeatRecompute(); });
  $('group-toggle').addEventListener('change', ()=>{ recomputeGroups(); });
  $('only-route').addEventListener('change', ()=>{ applyRouteFilter(); scheduleGroupRecompute(); scheduleHeatRecompute(); });
  $('recent-sec').addEventListener('change', ()=>{ applyRouteFilter(); scheduleGroupRecompute(); scheduleHeatRecompute(); });
  $('heat-toggle').addEventListener('change', ()=>{ document.getElementById('legend').style.display = $('heat-toggle').checked ? 'flex' : 'none'; recomputeHeat(); });

  function applyRouteFilter(){
    const only = $('only-route').checked;
    for (const [id, m] of Object.entries(driverMarkers)){
      const shouldShow = markerVisibleByFilters(id, only);
      const inLayer = driverLayer.hasLayer(m);
      if (shouldShow && !inLayer) driverLayer.addLayer(m);
      if (!shouldShow && inLayer) driverLayer.removeLayer(m);
    }
  }

  function markerVisibleByFilters(id, onlyRoute){
    const only = onlyRoute == null ? $('only-route').checked : onlyRoute;
    const recSec = parseInt($('recent-sec').value||'60',10);
    const last = driverLast.get(id) || 0;
    const fresh = (Date.now() - last) <= recSec*1000;
    const routeOk = !only || routeDrivers.has(id);
    return fresh && routeOk;
  }

  function scheduleHeatRecompute(){ if (heatDebounce) clearTimeout(heatDebounce); const n=Object.keys(driverMarkers).length; const delay = n>200?700:350; heatDebounce = setTimeout(recomputeHeat, delay); }
  function recomputeHeat(){
    heatLayer.clearLayers();
    if (!$('heat-toggle').checked) return;
    const recSec = parseInt($('recent-sec').value||'60',10);
    const now = Date.now();
    for (const [id, m] of Object.entries(driverMarkers)){
      const last = driverLast.get(id) || 0; const age = now - last;
      if (age > recSec*1000) continue;
      const pos = m.getLatLng();
      const alpha = Math.max(0.15, 0.8 * (1 - age/(recSec*1000)));
      const radius = 120; // meters
      const circle = L.circle(pos, { radius, color: `rgba(220,38,38,${alpha})`, weight: 2, fillColor: `rgba(220,38,38,${alpha*0.6})`, fillOpacity: alpha*0.6 });
      circle.addTo(heatLayer);
    }
  }

  setInterval(()=>{ if ($('heat-toggle').checked || $('only-route').checked) { applyRouteFilter(); scheduleGroupRecompute(); scheduleHeatRecompute(); } }, 5000);

  function addKnownDriver(id){ if (id) knownDrivers.add(id); }
  function refreshFollowList(){
    const sel = $('follow-driver'); if (!sel) return;
    const cur = sel.value;
    const opts = ['__ANY__', ...Array.from(knownDrivers).sort()];
    sel.innerHTML = '';
    for (const id of opts) {
      const o = document.createElement('option');
      o.value = id; o.textContent = (id === '__ANY__') ? 'Any' : id;
      sel.appendChild(o);
    }
    if (opts.includes(cur)) sel.value = cur;
    // re-render selected styles
    for (const [id, mrk] of Object.entries(driverMarkers)) {
      const pos = mrk.getLatLng();
      upsertDriverMarker(id, pos.lat, pos.lng);
    }
  }

  async function sendTestLocation() {
    try {
      const routeId = currentRouteId;
      if (!routeId) { alert('Subscribe to a route first.'); return; }
      let lat = 37.7749, lng = -122.4194;
      if (currentPath && currentPath.length > 0) {
        const i = Math.min( Math.floor(Math.random() * currentPath.length), currentPath.length - 1 );
        lat = currentPath[i].lat; lng = currentPath[i].lng;
      } else {
        const c = map.getCenter(); lat = c.lat; lng = c.lng;
      }
      const body = {
        tenantId: $('tenant').value.trim() || 't_demo',
        events: [{
          type: 'location', driverId: 'drv_demo', routeId,
          ts: new Date().toISOString(), payload: { lat, lng }
        }]
      };
      const res = await fetch('/v1/driver-events', { method: 'POST', headers: headers(), body: JSON.stringify(body) });
      if (!res.ok) throw new Error(await res.text());
      log({ event: 'test-location', lat, lng });
    } catch (e) { alert(`Send failed: ${e.message}`); }
  }

  function stopSimulation() {
    if (simTimer) { clearInterval(simTimer); simTimer = null; }
    const btn = $('btn-sim-toggle');
    if (btn) btn.textContent = 'Start Simulation';
    simDrivers = [];
  }

  function startSimulation() {
    const routeId = currentRouteId;
    if (!routeId) { alert('Subscribe to a route first.'); return; }
    const interval = Math.max(200, parseInt(($('sim-interval').value || '1500'), 10) || 1500);
    if (!currentPath || currentPath.length === 0 || pathTotal <= 0) {
      alert('No path available to simulate.');
      return;
    }
    const speedKmh = Math.max(1, parseFloat(($('sim-speed').value || '30')) || 30);
    const stepM = (speedKmh * 1000.0 / 3600.0) * (interval / 1000.0);
    const mode = ($('sim-mode').value || 'bounce');
    const nDrivers = Math.max(1, Math.min(20, parseInt(($('sim-drivers').value || '1'), 10) || 1));
    const jitterOn = $('sim-jitter').checked;
    const jitterM = Math.max(0, parseFloat(($('sim-jitter-m').value || '5')) || 0);
    // Initialize driver states evenly spaced along path
    simDrivers = [];
    for (let i = 0; i < nDrivers; i++) {
      const frac = nDrivers > 1 ? (i / (nDrivers)) : 0;
      const d0 = frac * pathTotal;
      simDrivers.push({ id: `drv_sim_${i+1}`, dist: d0, forward: true });
    }
    if (simTimer) clearInterval(simTimer);
    simTimer = setInterval(async () => {
      try {
        const events = [];
        for (let d of simDrivers) {
          // Advance distance
          if (mode === 'loop') {
            d.dist = (d.dist + stepM) % pathTotal;
          } else { // bounce
            if (d.forward) { d.dist += stepM; if (d.dist >= pathTotal) { d.dist = pathTotal; d.forward = false; } }
            else { d.dist -= stepM; if (d.dist <= 0) { d.dist = 0; d.forward = true; } }
          }
          const p = interpAtDistance(d.dist);
          const jittered = jitterOn ? jitterPoint(p.lat, p.lng, jitterM) : p;
          events.push({ type: 'location', driverId: d.id, routeId, ts: new Date().toISOString(), payload: { lat: jittered.lat, lng: jittered.lng } });
        }
        if (events.length > 0) {
          const body = { tenantId: $('tenant').value.trim() || 't_demo', events };
          await fetch('/v1/driver-events', { method: 'POST', headers: headers(), body: JSON.stringify(body) });
        }
      } catch (e) { /* ignore transient failures */ }
    }, interval);
    const btn = $('btn-sim-toggle');
    if (btn) btn.textContent = 'Stop Simulation';
  }

  function toggleSimulation() {
    if (simTimer) { stopSimulation(); } else { startSimulation(); }
  }

  async function optimizeDemo() {
    const today = new Date().toISOString().slice(0,10);
    const body = { tenantId: $('tenant').value.trim() || 't_demo', planDate: today, algorithm: 'greedy' };
    try {
      const res = await fetch('/v1/optimize', { method: 'POST', headers: headers(), body: JSON.stringify(body) });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      log({ event: 'optimize', batchId: data.batchId, routes: (data.routes||[]).map(r=>r.id) });
      await listRoutes();
    } catch (e) { alert(`Optimize failed: ${e.message}`); }
  }

  $('btn-refresh').onclick = refreshGeofences;
  $('btn-add').onclick = () => setAddMode(!addMode);
  $('btn-routes').onclick = listRoutes;
  $('btn-opt').onclick = optimizeDemo;
  $('btn-test-loc').onclick = sendTestLocation;
  $('btn-sim-toggle').onclick = toggleSimulation;
  $('btn-clear-drivers').onclick = () => { stopSimulation(); driverLayer.clearLayers(); driverMarkers = {}; };
  $('tenant').onchange = () => { refreshGeofences(); closeSSE(); $('routes').innerHTML=''; };

  // --- Geometry helpers ---
  function deg2rad(x){ return x*Math.PI/180; }
  function haversineM(lat1,lng1,lat2,lng2){
    const R=6371000; const dLat=deg2rad(lat2-lat1); const dLng=deg2rad(lng2-lng1);
    const a=Math.sin(dLat/2)**2 + Math.cos(deg2rad(lat1))*Math.cos(deg2rad(lat2))*Math.sin(dLng/2)**2;
    const c=2*Math.atan2(Math.sqrt(a), Math.sqrt(1-a)); return R*c;
  }
  function interpAtDistance(d){
    if (!currentPath || currentPath.length === 0) return { lat: 0, lng: 0 };
    if (currentPath.length === 1) return { lat: currentPath[0].lat, lng: currentPath[0].lng };
    if (d <= 0) return { lat: currentPath[0].lat, lng: currentPath[0].lng };
    if (d >= pathTotal) return { lat: currentPath[currentPath.length-1].lat, lng: currentPath[currentPath.length-1].lng };
    // find segment index where pathCum[i] <= d < pathCum[i+1]
    let i = 0;
    while (i < pathCum.length-1 && pathCum[i+1] < d) i++;
    const a = currentPath[i], b = currentPath[i+1];
    const segLen = pathCum[i+1] - pathCum[i];
    const frac = segLen > 0 ? (d - pathCum[i]) / segLen : 0;
    return { lat: a.lat + (b.lat - a.lat) * frac, lng: a.lng + (b.lng - a.lng) * frac };
  }
  function jitterPoint(lat,lng,meters){
    if (meters <= 0) return { lat, lng };
    const dLat = ( (Math.random()-0.5) * 2 * meters) / 111320;
    const dLng = ( (Math.random()-0.5) * 2 * meters) / (111320 * Math.cos(deg2rad(lat)) || 1);
    return { lat: lat + dLat, lng: lng + dLng };
  }

  // Initial
  refreshGeofences();
})();

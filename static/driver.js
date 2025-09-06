(function(){
  const $ = (sel)=>document.querySelector(sel);
  function saveCfg(){ localStorage.setItem('tenant',$('#tenant').value); localStorage.setItem('driverId',$('#driverId').value); localStorage.setItem('devToken',$('#devToken').checked?'1':'0'); }
  function loadCfg(){ $('#tenant').value = localStorage.getItem('tenant')||'t_demo'; $('#driverId').value = localStorage.getItem('driverId')||'drv1'; $('#devToken').checked = (localStorage.getItem('devToken')||'1')==='1'; }
  function headers(){ const t=$('#tenant').value.trim(); const d=$('#driverId').value.trim(); const h={'Content-Type':'application/json','X-Tenant-Id':t,'X-Role':'driver','X-Driver-Id':d}; if ($('#devToken').checked) h['Authorization']='Bearer '+t+':driver'; return h; }
  async function post(path, body){ const res = await fetch(path,{method:'POST',headers:headers(),body:body?JSON.stringify(body):undefined}); const txt = await res.text(); try{ return JSON.parse(txt); }catch{ return {raw:txt,status:res.status}; } }
  async function get(path){ const res = await fetch(path,{headers:headers()}); const txt = await res.text(); try{ return JSON.parse(txt); }catch{ return {raw:txt,status:res.status}; } }

  // HOS buttons
  $('#hos').addEventListener('click', async (e)=>{
    if(e.target.tagName!=='BUTTON') return;
    const action = e.target.getAttribute('data-action');
    const d = await post(`/v1/drivers/${encodeURIComponent($('#driverId').value)}/${action}`, {ts:new Date().toISOString()});
    $('#hosOut').textContent = JSON.stringify(d,null,2);
    updateHOSMini(d);
    showToast('HOS updated');
  });

  // Route
  $('#loadRoute').onclick = async ()=>{
    const id = $('#routeId').value.trim(); if(!id) return;
    const r = await get(`/v1/routes/${encodeURIComponent(id)}?includeBreaks=true`);
    $('#routeOut').textContent = JSON.stringify(r,null,2);
    renderNextStop(r); renderStops(r); cacheRoute(r);
    drawRouteMap(id);
  };
  $('#advance').onclick = async ()=>{
    const id=$('#routeId').value.trim(); if(!id) return; const reason = $('#reason').value || undefined;
    const r = await post(`/v1/routes/${encodeURIComponent(id)}/advance`, reason?{reason}:{});
    $('#routeOut').textContent = JSON.stringify(r,null,2);
    if (r && r.route) { renderNextStop(r.route); renderStops(r.route); showToast('Advanced'); if (navigator.vibrate) navigator.vibrate(40); setTimeout(()=>{ const btn = document.getElementById('btnArrive') || document.getElementById('btnDepart'); if (btn) btn.focus(); }, 10); }
  };

  let es;
  let reconnDelay = 2000; // ms
  let reconnTicker = null;
  $('#connectSSE').onclick = ()=>{
    if (es) { es.close(); es=null; }
    const id = $('#routeId').value.trim(); if(!id) return;
    const hdrs = headers();
    const url = `/v1/routes/${encodeURIComponent(id)}/events/stream`;
    const banner = document.getElementById('reconn'); if (banner) { banner.classList.remove('show'); }
    if (reconnTicker) { clearInterval(reconnTicker); reconnTicker = null; }
    reconnDelay = 2000;
    fetch(url,{headers:hdrs}).then(async (res)=>{
      const reader = res.body.getReader(); const dec = new TextDecoder(); let buf='';
      function push(){
        const segments = buf.split('\n\n');
        const filtered = [];
        for (const seg of segments){ if (seg.indexOf('event: heartbeat') !== -1) continue; filtered.push(seg); }
        $('#eventsOut').textContent = filtered.slice(-50).join('\n\n');
      }
      (async function pump(){
        try{
          for(;;){ const {done,value} = await reader.read(); if(done) break; const chunk = dec.decode(value,{stream:true}); buf += chunk; push(); handleSSEChunk(chunk); }
        }catch(e){ /* ignore */ }
        if ($('#autoRefresh').checked){
          if (banner) {
            banner.classList.add('show');
            let remain = Math.floor(reconnDelay/1000);
            banner.firstChild.nodeValue = `Reconnecting in ${remain}s… `;
            const btn = document.getElementById('btnRetrySSE');
            if (btn) {
              btn.onclick = ()=>{ if (reconnTicker) { clearInterval(reconnTicker); reconnTicker=null; } banner.classList.remove('show'); $('#connectSSE').click(); };
            }
            reconnTicker = setInterval(()=>{ remain--; if (remain>=0) banner.textContent = `Reconnecting in ${remain}s…`; }, 1000);
          }
          setTimeout(()=>{ if (banner) { banner.classList.remove('show'); banner.textContent=''; } if (reconnTicker) { clearInterval(reconnTicker); reconnTicker=null; } $('#connectSSE').click(); }, reconnDelay);
          reconnDelay = Math.min(reconnDelay * 2, 30000);
        }
      })();
    }).catch(()=>{ if ($('#autoRefresh').checked) setTimeout(()=>$('#connectSSE').click(), 5000); });
  }

  $('#save').onclick = saveCfg;
  loadCfg();
  refreshHOS();
  loadCachedRoute();

  // Help toggle
  (function(){ const btn=document.getElementById('btnHelp'); const box=document.getElementById('helpShortcuts'); if (!btn||!box) return; btn.addEventListener('click', ()=>{ const on = box.classList.toggle('show'); btn.setAttribute('aria-expanded', on?'true':'false'); }); })();

  // Keyboard shortcuts (within Route section)
  document.addEventListener('keydown', (e)=>{
    const tag = (e.target && e.target.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea') return;
    const routeId = document.getElementById('routeId').value.trim();
    if (!routeId) return;
    switch ((e.key||'').toLowerCase()){
      case 'a': document.getElementById('reason').value='arrive'; document.getElementById('advance').click(); e.preventDefault(); break;
      case 'd': document.getElementById('reason').value='depart'; document.getElementById('advance').click(); e.preventDefault(); break;
      case 'n': document.getElementById('btnNavigate').click(); e.preventDefault(); break;
      case 'f': { const fe=document.getElementById('followMe'); if (fe){ fe.checked=!fe.checked; localStorage.setItem('followMe', fe.checked?'1':'0'); } e.preventDefault(); break; }
      case 'l': { const se=document.getElementById('shareLoc'); if (se){ se.checked=!se.checked; se.dispatchEvent(new Event('change')); } e.preventDefault(); break; }
    }
  });

  // Clear events button
  (function(){ const b=document.getElementById('btnClearEvents'); if (!b) return; b.addEventListener('click', ()=>{ const el=document.getElementById('eventsOut'); if (el) el.textContent=''; }); })();

  // Active routes for driver
  async function refreshRoutes(){
    const d = $('#driverId').value.trim(); if(!d) return;
    const res = await get(`/v1/drivers/${encodeURIComponent(d)}/routes/summary`);
    const ul = $('#routesList'); ul.innerHTML='';
    const sel = document.getElementById('routeSelect'); if (sel) sel.innerHTML = '<option value="">(select)</option>';
    (res.items||[]).forEach(it=>{
      const li=document.createElement('li');
      const a=document.createElement('a'); a.href='#'; a.textContent=it.id; a.onclick=(e)=>{e.preventDefault(); $('#routeId').value=it.id; $('#loadRoute').click();};
      const st=document.createElement('span'); st.className='pill'; st.textContent=it.status||'';
      li.appendChild(st); li.appendChild(a);
      if (it.next && it.next.toStopId){ const ns=document.createElement('span'); ns.style.marginLeft='6px'; ns.textContent=`→ ${it.next.toStopId}`; li.appendChild(ns); }
      ul.appendChild(li);
      if (sel){ const opt=document.createElement('option'); opt.value=it.id; opt.textContent=it.id; sel.appendChild(opt); }
    });
    if (sel) sel.onchange = ()=>{ if (sel.value) { document.getElementById('routeId').value = sel.value; document.getElementById('loadRoute').click(); }}
  }

  async function refreshHOS(){
    const d = $('#driverId').value.trim(); if(!d) return;
    const res = await get(`/v1/drivers/${encodeURIComponent(d)}/hos`);
    updateHOSMini(res);
  }
  function updateHOSMini(res){ try{
    const el=document.getElementById('hosMini');
    if (!res || !el) return;
    const st = res.status || (res.hosState && res.hosState.status) || 'off';
    const br = res.hosState && res.hosState.break ? ' • on break' : '';
    el.textContent = `HOS: ${st}${br}`;
  }catch{} }
  $('#refreshRoutes').onclick = refreshRoutes;

  function renderNextStop(route){
    try{
      const card = document.getElementById('nextStop');
      if (!route || !route.legs || !route.legs.length){ card.style.display='none'; return; }
      const next = route.legs.find(l=>l.status==='in_progress') || route.legs.find(l=>l.status===''||!l.status);
      if (!next){ card.style.display='none'; return; }
      card.style.display='block';
      card.innerHTML = `<h3>Next Segment</h3>
        <div><span class="pill">From</span> ${next.fromStopId||'(depot)'} → <span class="pill">To</span> ${next.toStopId||'(depot)'}</div>
        <div><span class="pill">Kind</span> ${next.kind||'drive'} <span class="pill">Status</span> ${next.status||''}</div>
        <div id="etaLine"></div>
        <div style="margin-top:6px">
          <button id="btnArrive">Arrive</button>
          <button id="btnDepart">Depart</button>
        </div>`;
      document.getElementById('btnArrive').onclick = async ()=>{ $('#reason').value='arrive'; $('#advance').click(); };
      document.getElementById('btnDepart').onclick = async ()=>{ $('#reason').value='depart'; $('#advance').click(); };
      const podStop = document.getElementById('podStopId'); if (podStop && next.toStopId) podStop.value = next.toStopId;

      const etaA = next.etaArrival ? Date.parse(next.etaArrival) : null;
      const etaD = next.etaDeparture ? Date.parse(next.etaDeparture) : null;
      const line = document.getElementById('etaLine');
      function fmt(ms){ if(ms==null) return ''; let s=Math.max(0,Math.floor(ms/1000)); const h=Math.floor(s/3600); s%=3600; const m=Math.floor(s/60); s%=60; return `${h}h ${m}m ${s}s`; }
      function tick(){
        const now=Date.now();
        let html='';
        if (etaA){ const untilArr=etaA-now; html += `<span class="pill">Until arrival</span> ${fmt(untilArr)} `; if (untilArr<0){ html += `<span class="pill" style="background:#fecaca">Late</span> ${fmt(-untilArr)}`; } }
        if (etaD){ const untilDep=etaD-now; html += `<span class=\"pill\">Ready to depart</span> ${fmt(untilDep)}`; }
        line.innerHTML = html;
        const sum = document.getElementById('etaSummary'); if (sum){
          if (etaA){ const d=etaA-now; sum.textContent = d>=0?`Next arrival in ${fmt(d)}`:`Late by ${fmt(-d)}`; sum.className = 'eta-summary '+(d>=0?'eta-ok':'eta-late'); }
          else if (etaD){ const d=etaD-now; sum.textContent = `Ready to depart in ${fmt(d)}`; sum.className='eta-summary'; }
          else { sum.textContent=''; sum.className='eta-summary'; }
        }
      }
      // Pulse when ETA changes
      (function etaPulse(){ try{ const key='__eta_prev'; const cur=(etaA||etaD||0).toString(); const prev=card.getAttribute(key); if (prev && prev!==cur){ card.classList.add('eta-pulse'); setTimeout(()=>card.classList.remove('eta-pulse'), 900); } card.setAttribute(key, cur); }catch{} })();
      tick();
      if (window._etaTimer) clearInterval(window._etaTimer);
      window._etaTimer = setInterval(tick,1000);
    }catch(e){ /* ignore */ }
  }

  function renderStops(route){
    const ul = document.getElementById('stopList'); ul.innerHTML='';
    if (!route || !route.legs || route.legs.length===0) return;
    const curr = route.legs.find(l=>l.status==='in_progress');
    route.legs.forEach((l,i)=>{
      const li = document.createElement('li');
      const st = document.createElement('i'); st.className='status-dot '+(l.status==='in_progress'?'st-inprog':(l.status==='visited'?'st-visited':'st-pending'));
      const label = document.createElement('span'); label.textContent = `#${l.seq} → ${l.toStopId||'(depot)'} (${l.kind||'drive'})`;
      const btnA = document.createElement('button'); btnA.textContent='Arrive'; btnA.disabled = !(curr && curr.id===l.id);
      const btnD = document.createElement('button'); btnD.textContent='Depart'; btnD.disabled = !(curr && curr.id===l.id);
      btnA.onclick = ()=>{ document.getElementById('reason').value='arrive'; document.getElementById('advance').click(); };
      btnD.onclick = ()=>{ document.getElementById('reason').value='depart'; document.getElementById('advance').click(); };
      li.appendChild(st); li.appendChild(label); li.appendChild(btnA); li.appendChild(btnD);
      ul.appendChild(li);
    });
  }

  function showToast(msg, kind){ const t=document.getElementById('toast'); t.textContent=msg; t.classList.remove('danger'); if (kind==='danger') t.classList.add('danger'); t.classList.add('show'); setTimeout(()=>{ t.classList.remove('show'); t.classList.remove('danger'); t.textContent=''; }, 1600); }

  // --- Map ---
  let map, routeLayer, meMarker, nextMarker;
  function ensureMap(){
    if (map) return;
    map = L.map('dmap').setView([37.7749, -122.4194], 12);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', { attribution: '&copy; OpenStreetMap contributors' }).addTo(map);
    routeLayer = L.layerGroup().addTo(map);
  }
  async function drawRouteMap(routeId){
    ensureMap(); routeLayer.clearLayers(); if (meMarker) meMarker.remove(); if (nextMarker) nextMarker.remove();
    try{
      const data = await get(`/v1/routes/${encodeURIComponent(routeId)}/path`);
      const pts = (data && data.points)||[]; if (pts.length>0){ const latlngs=pts.map(p=>[p.lat,p.lng]); const poly=L.polyline(latlngs,{color:'#2563eb',weight:5}); poly.addTo(routeLayer); map.fitBounds(poly.getBounds(),{padding:[30,30]}); }
      const nd = await get(`/v1/routes/${encodeURIComponent(routeId)}/next-destination`);
      if (nd && nd.lat && nd.lng){ nextMarker = L.marker([nd.lat, nd.lng], { title: nd.stopId||'next' }).addTo(routeLayer); }
    }catch{}
  }
  function handleSSEChunk(chunk){
    // crude parse for driver.location events
    const lines = chunk.split('\n');
    let ev=null, data=null;
    for (const ln of lines){
      if (ln.startsWith('event: ')) ev = ln.slice(7).trim();
      if (ln.startsWith('data: ')) data = ln.slice(6).trim();
    }
    if (ev==='driver.location' && data){ try{ const d=JSON.parse(data); updateMe(d.lat,d.lng); }catch{} }
    if (ev==='stop.advanced' && data){ try{ showToast('Stop advanced'); if (navigator.vibrate) navigator.vibrate(30); }catch{} }
    // Append compact timeline for key events only
    try{
      if (ev && data && ev!=='heartbeat'){
        const out = document.getElementById('eventsOut'); if (!out) return;
        let msg = ev;
        try{ const obj=JSON.parse(data); if (ev==='stop.advanced'){ msg += ` ${obj.fromStopId||''} → ${obj.toStopId||''}`; } }catch{}
        const ts = new Date().toISOString();
        out.textContent = (out.textContent + `\n${ts} ${msg}`).split('\n').slice(-50).join('\n');
        out.scrollTop = out.scrollHeight;
      }
    }catch{}
  }
  function updateMe(lat,lng){
    ensureMap();
    const icon = L.divIcon({ className: '', html: '<i class="me-pin pulse"></i>', iconSize: [14,14], iconAnchor: [7,7] });
    if (!meMarker){ meMarker=L.marker([lat,lng],{title:'me', icon}).addTo(routeLayer); } else { meMarker.setLatLng([lat,lng]); meMarker.setIcon(icon); }
    var fm = document.getElementById('followMe'); if (fm && fm.checked) { map.panTo([lat,lng], {animate:true}); }
  }

  // Re-center Map button
  const btnRC = document.getElementById('btnRecenter');
  if (btnRC) btnRC.addEventListener('click', async ()=>{
    try{
      const id = document.getElementById('routeId').value.trim(); if(!id) return;
      const data = await get(`/v1/routes/${encodeURIComponent(id)}/path`);
      const pts = (data && data.points)||[]; if (pts.length>0){ const latlngs=pts.map(p=>[p.lat,p.lng]); const poly=L.polyline(latlngs); map.fitBounds(poly.getBounds(),{padding:[30,30]}); return; }
      if (meMarker){ map.panTo(meMarker.getLatLng(), {animate:true}); }
    }catch{}
  });

  // Offline storage (IndexedDB with localStorage fallback)
  const idb = ('indexedDB' in window) ? window.indexedDB : null;
  function idbOpen(){ return new Promise((resolve,reject)=>{ if(!idb) return reject('no idb'); const req = idb.open('driver',1); req.onupgradeneeded=(e)=>{ const db=e.target.result; if(!db.objectStoreNames.contains('routes')) db.createObjectStore('routes',{keyPath:'id'}); if(!db.objectStoreNames.contains('outbox')) db.createObjectStore('outbox',{autoIncrement:true}); }; req.onsuccess=()=>resolve(req.result); req.onerror=()=>reject(req.error); }); }
  async function idbPut(store,obj){ try{ const db=await idbOpen(); return await new Promise((resolve,reject)=>{ const tx=db.transaction(store,'readwrite'); tx.objectStore(store).put(obj); tx.oncomplete=()=>resolve(); tx.onerror=()=>reject(tx.error); }); }catch{ /* ignore */ } }
  async function idbGet(store,key){ try{ const db=await idbOpen(); return await new Promise((resolve,reject)=>{ const tx=db.transaction(store,'readonly'); const req=tx.objectStore(store).get(key); req.onsuccess=()=>resolve(req.result||null); req.onerror=()=>reject(req.error); }); }catch{ return null } }
  async function idbAll(store){ try{ const db=await idbOpen(); return await new Promise((resolve,reject)=>{ const tx=db.transaction(store,'readonly'); const req=tx.objectStore(store).getAll(); req.onsuccess=()=>resolve(req.result||[]); req.onerror=()=>reject(req.error); }); }catch{ return [] } }
  async function idbClear(store){ try{ const db=await idbOpen(); return await new Promise((resolve,reject)=>{ const tx=db.transaction(store,'readwrite'); const req=tx.objectStore(store).clear(); req.onsuccess=()=>resolve(); req.onerror=()=>reject(req.error); }); }catch{ /* ignore */ } }

  async function cacheRoute(route){ try{ if (route && route.id){ localStorage.setItem('lastRouteId', route.id); await idbPut('routes', route); localStorage.setItem('route:'+route.id, JSON.stringify(route)); } }catch{} }
  async function loadCachedRoute(){ try{ const id = localStorage.getItem('lastRouteId'); if (!id) return false; const r = (await idbGet('routes', id)) || JSON.parse(localStorage.getItem('route:'+id)||'null'); if (!r) return false; document.getElementById('routeId').value = id; document.getElementById('routeOut').textContent = JSON.stringify(r,null,2); renderNextStop(r); renderStops(r); drawRouteMap(id); return true; }catch{ return false } }

  // Outbox via IDB (fallback to localStorage helpers from earlier)
  async function outboxAdd(ev){ try{ await idbPut('outbox', ev); }catch{ const box=loadOutbox(); box.push(ev); saveOutbox(box); } }
  async function outboxDrain(){ try{ const items = await idbAll('outbox'); await idbClear('outbox'); return items; }catch{ const box=loadOutbox(); saveOutbox([]); return box; } }

  // Periodic auto-refresh
  setInterval(()=>{ if ($('#autoRefresh').checked) { refreshRoutes(); if (!es) $('#connectSSE').click(); } }, 15000);

  // PoD submission
  document.getElementById('btnPod').onclick = async ()=>{
    const tenant = $('#tenant').value.trim();
    const orderId = document.getElementById('podOrderId').value.trim();
    const stopId  = document.getElementById('podStopId').value.trim();
    const type    = document.getElementById('podType').value;
    const file    = document.getElementById('podFile').files[0];
    const errEl = document.getElementById('podErr'); if (errEl) errEl.textContent='';
    const maxBytes = 15*1024*1024; const allowed = ['image/jpeg','image/png','image/webp'];
    if (file){ if (file.size > maxBytes){ if (errEl) errEl.textContent='File too large (max 15MB).'; return; } if (allowed.indexOf(file.type)===-1){ if (errEl) errEl.textContent='Unsupported type. Use jpg, png, or webp.'; return; } }
    let sha = '';
    try{
      if (file){
        const buf = await file.arrayBuffer();
        const hash = await crypto.subtle.digest('SHA-256', buf);
        sha = Array.from(new Uint8Array(hash)).map(b=>b.toString(16).padStart(2,'0')).join('');
        const presign = await post('/v1/media/presign', {tenantId: tenant, fileName: file.name, contentType: file.type||'application/octet-stream', bytes: file.size, sha256: sha});
        if (presign && presign.uploadUrl){
          const headers = presign.headers||{};
          await fetch(presign.uploadUrl, {method: presign.method||'PUT', headers: headers, body: file});
          const note = (document.getElementById('podNote')||{}).value || '';
          const payload = {tenantId: tenant, orderId, stopId, type, media:{uploadUrl: presign.uploadUrl, sha256: sha}, metadata:{filename:file.name, note}};
          const res = await post('/v1/pod', payload);
          document.getElementById('podOut').textContent = JSON.stringify(res,null,2);
          return;
        }
      }
    }catch(e){ if (errEl) errEl.textContent = 'Upload failed, submitting metadata only.'; }
    const note = (document.getElementById('podNote')||{}).value || '';
    const payload = {tenantId: tenant, orderId, stopId, type, metadata:{note}};
    const res = await post('/v1/pod', payload);
    document.getElementById('podOut').textContent = JSON.stringify(res,null,2);
    if (res && !res.status) { showToast('PoD submitted'); if (navigator.vibrate) navigator.vibrate(40); } else { showToast('PoD error','danger'); }
  };

  // PoD preview
  (function initPreview(){ var el=document.getElementById('podFile'); if (!el) return; el.addEventListener('change', function(){ var pv=document.getElementById('podPreview'); if (!pv) return; pv.innerHTML=''; var f=el.files && el.files[0]; if (!f) return; if (f.type && f.type.indexOf('image/')===0){ var img=new Image(); img.onload=function(){ URL.revokeObjectURL(img.src); }; img.src=URL.createObjectURL(f); pv.appendChild(img); } }); })();

  // Offline route cache
  function cacheRoute(route){ try{ if (route && route.id){ localStorage.setItem('lastRouteId', route.id); localStorage.setItem('route:'+route.id, JSON.stringify(route)); } }catch{}
  }
  function loadCachedRoute(){ try{ const id = localStorage.getItem('lastRouteId'); if (!id) return false; const raw = localStorage.getItem('route:'+id); if (!raw) return false; const r = JSON.parse(raw); document.getElementById('routeId').value = id; document.getElementById('routeOut').textContent = JSON.stringify(r,null,2); renderNextStop(r); renderStops(r); drawRouteMap(id); return true; }catch{ return false }
  }

  // Navigate to next destination
  document.getElementById('btnNavigate').onclick = async ()=>{
    const id = $('#routeId').value.trim(); if(!id) return;
    const d = await get(`/v1/routes/${encodeURIComponent(id)}/next-destination`);
    if (!d || d.status>=400) { alert('No destination'); return; }
    if (d.lat && d.lng){
      const lat = d.lat, lng = d.lng;
      const isApple = /iPhone|iPad|Macintosh/.test(navigator.userAgent);
      const url = isApple ? `http://maps.apple.com/?daddr=${lat},${lng}` : `https://www.google.com/maps/dir/?api=1&destination=${lat},${lng}`;
      window.open(url, '_blank');
    } else {
      alert('Destination coordinates unavailable.');
    }
  };

  // Share location
  let watchId=null, lastSent=0;
  function stopShare(){ if (watchId!=null) { navigator.geolocation.clearWatch(watchId); watchId=null; } }
  // simple outbox in localStorage for offline driver-events
  function loadOutbox(){ try{ return JSON.parse(localStorage.getItem('outbox')||'[]'); }catch{return []} }
  function saveOutbox(arr){ localStorage.setItem('outbox', JSON.stringify(arr)); }
  async function flushOutbox(){ const box = loadOutbox(); if (box.length===0) return; try{ const body={ tenantId: $('#tenant').value.trim(), events: box }; await fetch('/v1/driver-events',{method:'POST', headers: headers(), body: JSON.stringify(body)}); saveOutbox([]); }catch{} }
  window.addEventListener('online', flushOutbox);
  // Also drain IDB outbox periodically and on reconnect
  async function flushIDBOutbox(){ try{ const items = await outboxDrain(); if (!items || items.length===0) return; const body={ tenantId: $('#tenant').value.trim(), events: items }; await fetch('/v1/driver-events',{method:'POST', headers: headers(), body: JSON.stringify(body)}); }catch{} }
  window.addEventListener('online', flushIDBOutbox);
  setInterval(flushIDBOutbox, 30000);

  // Restore persisted toggles for sharing and following
  (function restoreToggles(){ try{ var fe=document.getElementById('followMe'); var se=document.getElementById('shareLoc'); if (fe){ fe.checked = localStorage.getItem('followMe')==='1'; fe.addEventListener('change', function(){ localStorage.setItem('followMe', fe.checked?'1':'0'); }); } if (se){ se.checked = localStorage.getItem('shareLoc')==='1'; se.addEventListener('change', function(){ localStorage.setItem('shareLoc', se.checked?'1':'0'); }); if (se.checked) se.dispatchEvent(new Event('change')); } }catch{} })();
  async function sendLoc(lat,lng){
    const now=Date.now(); const iv = (parseInt($('#shareInterval').value)||10)*1000;
    if (now - lastSent < iv) return; lastSent = now;
    const routeId = $('#routeId').value.trim();
    const ev = { type:'location', driverId: $('#driverId').value.trim(), routeId, ts: new Date().toISOString(), payload:{ lat, lng } };
    const body = { tenantId: $('#tenant').value.trim(), events: [ev] };
    try{ await fetch('/v1/driver-events', { method:'POST', headers: headers(), body: JSON.stringify(body) }); }
    catch{ await outboxAdd(ev); }
  }
  document.getElementById('shareLoc').addEventListener('change', (e)=>{
    if (e.target.checked) {
      if (!navigator.geolocation){ alert('Geolocation not supported'); e.target.checked=false; return; }
      watchId = navigator.geolocation.watchPosition((pos)=>{
        const {latitude:lat, longitude:lng} = pos.coords; sendLoc(lat,lng);
      }, ()=>{}, {enableHighAccuracy:true, maximumAge:5000, timeout:10000});
    } else { stopShare(); }
  });
})();

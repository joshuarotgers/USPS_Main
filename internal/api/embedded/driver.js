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
  });

  // Route
  $('#loadRoute').onclick = async ()=>{
    const id = $('#routeId').value.trim(); if(!id) return;
    const r = await get(`/v1/routes/${encodeURIComponent(id)}?includeBreaks=true`);
    $('#routeOut').textContent = JSON.stringify(r,null,2);
    renderNextStop(r);
  };
  $('#advance').onclick = async ()=>{
    const id=$('#routeId').value.trim(); if(!id) return; const reason = $('#reason').value || undefined;
    const r = await post(`/v1/routes/${encodeURIComponent(id)}/advance`, reason?{reason}:{});
    $('#routeOut').textContent = JSON.stringify(r,null,2);
    if (r && r.route) renderNextStop(r.route);
  };

  let es;
  $('#connectSSE').onclick = ()=>{
    if (es) { es.close(); es=null; }
    const id = $('#routeId').value.trim(); if(!id) return;
    const hdrs = headers();
    const url = `/v1/routes/${encodeURIComponent(id)}/events/stream`;
    // Use EventSource polyfill via fetch stream to include headers
    fetch(url,{headers:hdrs}).then(async (res)=>{
      const reader = res.body.getReader(); const dec = new TextDecoder(); let buf='';
      function push(){ $('#eventsOut').textContent = buf.split('\n\n').slice(-50).join('\n\n'); }
      (async function pump(){
        try{
          for(;;){ const {done,value} = await reader.read(); if(done) break; buf += dec.decode(value,{stream:true}); push(); }
        }catch(e){ /* ignore */ }
        // Auto-reconnect if toggle enabled
        if ($('#autoRefresh').checked) setTimeout(()=>$('#connectSSE').click(), 2000);
      })();
    }).catch(()=>{ if ($('#autoRefresh').checked) setTimeout(()=>$('#connectSSE').click(), 5000); });
  }

  $('#save').onclick = saveCfg;
  loadCfg();

  // Active routes for driver
  async function refreshRoutes(){
    const d = $('#driverId').value.trim(); if(!d) return;
    const res = await get(`/v1/drivers/${encodeURIComponent(d)}/routes/summary`);
    const ul = $('#routesList'); ul.innerHTML='';
    (res.items||[]).forEach(it=>{
      const li=document.createElement('li');
      const a=document.createElement('a'); a.href='#'; a.textContent=it.id; a.onclick=(e)=>{e.preventDefault(); $('#routeId').value=it.id; $('#loadRoute').click();};
      const st=document.createElement('span'); st.className='pill'; st.textContent=it.status||'';
      li.appendChild(st); li.appendChild(a);
      if (it.next && it.next.toStopId){ const ns=document.createElement('span'); ns.style.marginLeft='6px'; ns.textContent=`→ ${it.next.toStopId}`; li.appendChild(ns); }
      ul.appendChild(li);
    });
  }
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
      // prefill PoD stop
      const podStop = document.getElementById('podStopId'); if (podStop && next.toStopId) podStop.value = next.toStopId;

      // ETA/Dwell countdowns
      const etaA = next.etaArrival ? Date.parse(next.etaArrival) : null;
      const etaD = next.etaDeparture ? Date.parse(next.etaDeparture) : null;
      const line = document.getElementById('etaLine');
      function fmt(ms){ if(ms==null) return ''; let s=Math.max(0,Math.floor(ms/1000)); const h=Math.floor(s/3600); s%=3600; const m=Math.floor(s/60); s%=60; return `${h}h ${m}m ${s}s`; }
      function tick(){
        const now=Date.now();
        let html='';
        if (etaA){ const untilArr=etaA-now; html += `<span class="pill">Until arrival</span> ${fmt(untilArr)} `; }
        if (etaD){ const untilDep=etaD-now; html += `<span class=\"pill\">Ready to depart</span> ${fmt(untilDep)}`; }
        line.innerHTML = html;
      }
      tick();
      if (window._etaTimer) clearInterval(window._etaTimer);
      window._etaTimer = setInterval(tick,1000);
    }catch(e){ /* ignore */ }
  }

  // Periodic auto-refresh of routes list and SSE reconnect
  setInterval(()=>{ if ($('#autoRefresh').checked) { refreshRoutes(); if (!es) $('#connectSSE').click(); } }, 15000);

  // PoD submission
  document.getElementById('btnPod').onclick = async ()=>{
    const tenant = $('#tenant').value.trim();
    const orderId = document.getElementById('podOrderId').value.trim();
    const stopId  = document.getElementById('podStopId').value.trim();
    const type    = document.getElementById('podType').value;
    const file    = document.getElementById('podFile').files[0];
    let sha = '';
    try{
      if (file){
        const buf = await file.arrayBuffer();
        const hash = await crypto.subtle.digest('SHA-256', buf);
        sha = Array.from(new Uint8Array(hash)).map(b=>b.toString(16).padStart(2,'0')).join('');
        // presign
        const presign = await post('/v1/media/presign', {tenantId: tenant, fileName: file.name, contentType: file.type||'application/octet-stream', bytes: file.size, sha256: sha});
        if (presign && presign.uploadUrl){
          const headers = presign.headers||{};
          await fetch(presign.uploadUrl, {method: presign.method||'PUT', headers: headers, body: file});
          // now create PoD referencing uploaded media
          const payload = {tenantId: tenant, orderId, stopId, type, media:{uploadUrl: presign.uploadUrl, sha256: sha}, metadata:{filename:file.name}};
          const res = await post('/v1/pod', payload);
          document.getElementById('podOut').textContent = JSON.stringify(res,null,2);
          return;
        }
      }
    }catch(e){ /* fallback to metadata-only */ }
    const payload = {tenantId: tenant, orderId, stopId, type, metadata:{}};
    const res = await post('/v1/pod', payload);
    document.getElementById('podOut').textContent = JSON.stringify(res,null,2);
  };
})();

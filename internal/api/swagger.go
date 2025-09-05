package api

import (
    "encoding/base64"
    "encoding/json"
    "net/http"

    yaml "gopkg.in/yaml.v3"
)

// SwaggerHandler serves an interactive Swagger UI with inlined spec and auth presets.
func (s *Server) SwaggerHandler(w http.ResponseWriter, r *http.Request) {
    data, err := openAPILoad()
    if err != nil { writeProblem(w, 500, "OpenAPI not available", err.Error(), r.URL.Path); return }
    var obj map[string]any
    if err := yaml.Unmarshal(data, &obj); err != nil { writeProblem(w, 500, "OpenAPI parse failed", err.Error(), r.URL.Path); return }
    js, _ := json.Marshal(obj)
    b64 := base64.StdEncoding.EncodeToString(js)
    html := `<!DOCTYPE html><html lang="en"><head>
    <title>API Console</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <link rel="stylesheet" href="/static/swagger-ui.css" />
    <style>body{margin:0} .topbar{display:none} .cfg{position:fixed;top:8px;right:8px;padding:8px;background:#fff;border:1px solid #ddd;z-index:9}</style>
    </head><body>
    <div class="cfg">
      <div><strong>Auth Presets</strong></div>
      <div><label>Tenant: <input id="tenant" value="t_demo"></label></div>
      <div><label>Role: <input id="role" value="admin"></label></div>
      <div><label>Bearer token: <input id="token" style="width:240px"></label></div>
      <div><label><input type="checkbox" id="useDev"> Use dev tenant:role token</label></div>
      <button onclick="saveAuth()">Save</button>
    </div>
    <div id="swagger-ui"></div>
    <script src="/static/swagger-ui-bundle.js"></script>
    <script src="/static/swagger-ui-standalone-preset.js"></script>
    <script>
    const spec = JSON.parse(atob('` + b64 + `'));
    function loadAuth(){
      const t=localStorage.getItem('tenant')||''; const r=localStorage.getItem('role')||''; const k=localStorage.getItem('token')||''; const d=localStorage.getItem('useDev')==='1';
      document.getElementById('tenant').value=t; document.getElementById('role').value=r; document.getElementById('token').value=k; document.getElementById('useDev').checked=d;
      return {tenant:t, role:r, token:k, useDev:d};
    }
    function saveAuth(){ const t=document.getElementById('tenant').value; const r=document.getElementById('role').value; const k=document.getElementById('token').value; const d=document.getElementById('useDev').checked; localStorage.setItem('tenant',t); localStorage.setItem('role',r); localStorage.setItem('token',k); localStorage.setItem('useDev',d?'1':'0'); alert('Saved'); }
    const presetAuth = loadAuth();
    const ui = SwaggerUIBundle({
        spec: spec,
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        layout: "BaseLayout",
        requestInterceptor: (req) => {
            const p = loadAuth();
            if (p.useDev && p.tenant && p.role) { req.headers['Authorization'] = 'Bearer ' + p.tenant + ':' + p.role; }
            else if (p.token) { req.headers['Authorization'] = 'Bearer ' + p.token; }
            if (p.tenant) req.headers['X-Tenant-Id'] = p.tenant;
            if (p.role) req.headers['X-Role'] = p.role;
            return req;
        }
    });
    </script>
    </body></html>`
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _, _ = w.Write([]byte(html))
}

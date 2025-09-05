self.addEventListener('install', (e)=>{
  e.waitUntil(caches.open('driver-app-v1').then((c)=>c.addAll([
    '/app','/static/driver.css','/static/driver.js','/static/manifest.json'
  ])));
});
self.addEventListener('fetch', (e)=>{
  const url = new URL(e.request.url);
  if (url.pathname.startsWith('/static/') || url.pathname === '/app' || url.pathname === '/app/') {
    e.respondWith(caches.match(e.request).then((res)=>res||fetch(e.request)));
  }
});


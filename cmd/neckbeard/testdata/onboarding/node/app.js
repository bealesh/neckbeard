const http = require('node:http');
http.createServer((req, res) => {
  res.end(req.url === "/healthz" ? 'ok' : 'hello');
}).listen(3000);

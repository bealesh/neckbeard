// Detection fixture: Redis is required but cannot be provisioned by the catalog.
const redisURL = process.env.REDIS_URL;
const http = require('node:http');
http.createServer((req, res) => {
  res.end(req.url === "/healthz" ? 'ok' : 'hello');
}).listen(3000);

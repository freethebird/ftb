// Tiny static file server for local preview (no dependencies). The deployed page is plain
// static files — any web server or IPFS pin serves it; this exists only so `npm run dev`
// gives a local preview. Accepts --port/--host or PORT/PORT-style CLI args.
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const args = process.argv.slice(2);
function arg(name, dflt) {
  const i = args.findIndex(a => a === '--' + name || a === name);
  return i >= 0 && args[i + 1] ? args[i + 1] : dflt;
}
const port = Number(arg('port', process.env.PORT || 7100));
const host = arg('host', '127.0.0.1');

const MIME = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json', '.css': 'text/css', '.png': 'image/png',
  '.svg': 'image/svg+xml', '.ico': 'image/x-icon'
};

createServer(async (req, res) => {
  try {
    let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
    if (p.endsWith('/')) p += 'index.html';
    const file = normalize(join(root, p));
    if (!file.startsWith(root)) { res.writeHead(403); res.end(); return; }
    const data = await readFile(file);
    res.writeHead(200, { 'content-type': MIME[extname(file)] || 'application/octet-stream' });
    res.end(data);
  } catch {
    res.writeHead(404); res.end('not found');
  }
}).listen(port, host, () => console.log(`FTB dev preview: http://${host}:${port}/`));

import fs from 'fs';
const html = fs.readFileSync(process.argv[2], 'utf8');
const re = /\{ id: '([A-Z]\d+)'([\s\S]*?)src: \[([^\]]*)\] \}/g;
const want = new Set(process.argv.slice(4).flatMap(x => x.split(',')));
const out = [];
let m;
while ((m = re.exec(html))) {
  const id = m[1]; if (!want.has(id)) continue;
  const body = m[2];
  const get = k => { const r = new RegExp(k + ": '((?:[^'\\\\]|\\\\.)*)'"); const x = body.match(r); return x ? x[1].replace(/\\'/g, "'") : ''; };
  const s = (body.match(/s: (\d)/) || [])[1];
  let text = `**${id} (P${s}). ${get('t')}**\n`;
  if (get('d')) text += get('d') + '\n';
  if (get('c')) text += 'Likely cause: ' + get('c') + '\n';
  text += 'QA sources: ' + m[3].replace(/'/g, '') + '\n';
  out.push([id, text]);
}
const order = process.argv.slice(4).flatMap(x => x.split(','));
out.sort((a, b) => order.indexOf(a[0]) - order.indexOf(b[0]));
fs.writeFileSync(process.argv[3], out.map(x => x[1]).join('\n'));
console.log(process.argv[3], out.map(x => x[0]).join(' '));

#!/usr/bin/env node
const fs = require('node:fs');
const http = require('node:http');
const net = require('node:net');
const { spawnSync } = require('node:child_process');

const bridgeUrl = process.env.LIMRUN_ARGENT_BRIDGE_URL;
const device = JSON.parse(process.env.LIMRUN_ARGENT_DEVICE || 'null');
const udid = device.entry.udid;
const realXcrun = process.env.LIMRUN_REAL_XCRUN || '/usr/bin/xcrun';
const hasRealXcrun = fs.existsSync(realXcrun);
const args = process.argv.slice(2);

function delegate() {
  if (!hasRealXcrun) {
    process.stderr.write('limrun xcrun shim: unsupported without Xcode: xcrun ' + args.join(' ') + '\n');
    process.exit(127);
  }
  const result = spawnSync(realXcrun, args, { stdio: 'inherit' });
  process.exit(result.status ?? 1);
}

function post(route, body) {
  return new Promise((resolve, reject) => {
    const payload = JSON.stringify(body);
    const req = http.request(bridgeUrl + route, {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'content-length': Buffer.byteLength(payload) },
    }, (res) => {
      const chunks = [];
      res.on('data', (chunk) => chunks.push(chunk));
      res.on('end', () => {
        try {
          resolve(JSON.parse(Buffer.concat(chunks).toString('utf8')));
        } catch (error) {
          reject(error);
        }
      });
    });
    req.on('error', reject);
    req.end(payload);
  });
}

// \`simctl list devices --json\`: the local simulators plus the remote one.
function listDevices() {
  let listing = { devices: {} };
  if (hasRealXcrun) {
    const real = spawnSync(realXcrun, args, { encoding: 'utf8' });
    try {
      listing = JSON.parse(real.stdout);
    } catch {}
  }
  listing.devices = listing.devices || {};
  listing.devices[device.runtime] = [...(listing.devices[device.runtime] || []), device.entry];
  process.stdout.write(JSON.stringify(listing) + '\n');
  process.exit(0);
}

// Argent listens on a socket and spawns its accessibility daemon, which dials
// in. This process takes the daemon's place and answers from the bridge.
function accessibilityDaemon(endpoint) {
  const socket = net.connect(endpoint);
  let buffered = '';
  socket.on('data', (chunk) => {
    buffered += chunk.toString('utf8');
    let end;
    while ((end = buffered.indexOf('\n')) >= 0) {
      const line = buffered.slice(0, end).trim();
      buffered = buffered.slice(end + 1);
      if (!line) continue;
      const { id, command, params } = JSON.parse(line);
      post('/ax', { command, params })
        .catch((error) => ({ error: String(error.message || error) }))
        .then((reply) => socket.write(JSON.stringify({ id, ...reply }) + '\n'));
    }
  });
  socket.on('close', () => process.exit(0));
  socket.on('error', (error) => {
    process.stderr.write('limrun xcrun shim: ' + error.message + '\n');
    process.exit(1);
  });
}

function main() {
  if (args[0] !== 'simctl') return delegate();
  // Argent scopes simctl to a device set; the remote simulator has none here.
  const simctlArgs = args[1] === '--set' ? args.slice(3) : args.slice(1);
  const command = simctlArgs[0];
  const wantsJson = simctlArgs.includes('--json') || simctlArgs.includes('-j');
  if (command === 'list' && simctlArgs[1] === 'devices' && wantsJson) return listDevices();
  if (!simctlArgs.includes(udid)) return delegate();

  if (command === 'spawn' && /(^|\/)ax-service$/.test(simctlArgs[2] || '')) {
    const socketAt = simctlArgs.indexOf('--socket');
    const portAt = simctlArgs.indexOf('--port');
    if (socketAt > 0) return accessibilityDaemon({ path: simctlArgs[socketAt + 1] });
    if (portAt > 0) return accessibilityDaemon({ host: '127.0.0.1', port: Number(simctlArgs[portAt + 1]) });
  }
  post('/xcrun', { args: simctlArgs }).then((result) => {
    if (result.stdout) process.stdout.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    process.exit(typeof result.code === 'number' ? result.code : 1);
  }, (error) => {
    process.stderr.write('limrun xcrun shim: ' + error.message + '\n');
    process.exit(1);
  });
}

main();

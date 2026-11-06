#!/usr/bin/env node
const http = require('node:http');
const { spawnSync } = require('node:child_process');

const args = process.argv.slice(2);
const realXcrun = process.env.LIMRUN_REAL_XCRUN || '/usr/bin/xcrun';

function delegate() {
  const result = spawnSync(realXcrun, args, { stdio: 'inherit' });
  if (result.error) {
    process.stderr.write(String(result.error.message || result.error) + '\n');
    process.exit(127);
  }
  process.exit(result.status ?? 1);
}

function fail(message) {
  process.stderr.write('limrun xcrun shim: ' + message + '\n');
  process.exit(64);
}

// Physical-device enumeration (e.g. Maestro's device picker) is answered by
// the shim server; real devicectl does not exist off macOS. Every other
// devicectl subcommand delegates. Flags can precede the subcommand
// (devicectl --json-output <path> list devices), so match on positionals.
function devicectlPositionals(args) {
  const positionals = [];
  for (let i = 1; i < args.length; i++) {
    if (args[i] === '--json-output') {
      i++;
      continue;
    }
    if (args[i].startsWith('-')) continue;
    positionals.push(args[i]);
  }
  return positionals;
}
const devicectlSubcommand = args[0] === 'devicectl' ? devicectlPositionals(args) : [];
const isDevicectlListDevices = devicectlSubcommand[0] === 'list' && devicectlSubcommand[1] === 'devices';

if (args[0] !== 'simctl' && !isDevicectlListDevices) {
  delegate();
}

const shimUrl = process.env.LIMRUN_XCRUN_SHIM_URL;
const udid = process.env.LIMRUN_IOS_UDID;
if (!shimUrl || !udid) {
  fail('LIMRUN_XCRUN_SHIM_URL and LIMRUN_IOS_UDID are required.');
}

function simctlTarget(command, simctlArgs) {
  if (command === 'launch') {
    return simctlArgs.slice(1).find((arg) => !arg.startsWith('-'));
  }
  return simctlArgs[1];
}
function isLimrunTarget(value) {
  return value === udid || value === 'booted';
}

if (args[0] === 'simctl') {
  const simctlArgs = args.slice(1);
  const command = simctlArgs[0];
  const target = simctlTarget(command, simctlArgs);
  if (command !== 'list' && !isLimrunTarget(target)) {
    delegate();
  }
}

const parsed = new URL(shimUrl);
const body = JSON.stringify({ args });
const req = http.request(parsed, {
  method: 'POST',
  headers: {
    'content-type': 'application/json',
    'content-length': Buffer.byteLength(body),
  },
}, (res) => {
  const chunks = [];
  res.on('data', (chunk) => chunks.push(chunk));
  res.on('end', () => {
    let payload;
    try {
      payload = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    } catch (error) {
      process.stderr.write('limrun xcrun shim: invalid shim response\n');
      process.exit(1);
    }
    if (payload.stdout) process.stdout.write(payload.stdout);
    if (payload.stderr) process.stderr.write(payload.stderr);
    process.exit(typeof payload.code === 'number' ? payload.code : 1);
  });
});
req.on('error', (error) => {
  process.stderr.write('limrun xcrun shim: ' + error.message + '\n');
  process.exit(1);
});
req.end(body);

// @limrun/detox takes a TypeScript instance client; this adapter is the only
// example that also needs @limrun/api. Go owns instance creation/deletion.
import path from 'node:path';
import { Ios } from '@limrun/api';
import { prepareDetoxRun, runDetoxTest } from '@limrun/detox';
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));

const artifactDirectory = path.resolve(process.env.DETOX_ARTIFACTS_DIR || 'artifacts/limrun-detox');
const client = await Ios.createInstanceClient({ apiUrl: input.apiUrl, token: input.token });
let prepared;
try {
  prepared = await prepareDetoxRun({
    client,
    sessionId: process.env.DETOX_SESSION_ID || 'limrun-detox-example',
    artifactDirectory,
    version: process.env.DETOX_VERSION,
  });
  const result = await runDetoxTest({
    configPath: './.detoxrc.cjs',
    configuration: 'ios.limrun.expo-go',
    sessionId: prepared.sessionId,
    serverUrl: prepared.detoxServerUrl,
    iosId: input.id,
    iosApiUrl: input.apiUrl,
    iosToken: input.token,
    artifactDirectory: prepared.artifactDirectory,
    cwd: process.cwd(),
    extraEnv: {
      EXPO_URL: process.env.EXPO_URL,
      LIMRUN_DETOX_SERVER_URL: prepared.remoteDetoxServerUrl,
      DETOX_VERSION: prepared.version,
    },
  });
  if (result.code !== 0) throw new Error(`Detox exited with code ${result.code}`);
  console.log('Artifacts:', prepared.artifactDirectory);
  console.log('Screenshots: limrun-detox-{home,counter,greeting,success}');
} finally {
  try { await prepared?.cleanup(); } finally { client.disconnect(); }
}

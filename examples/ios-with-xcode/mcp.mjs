// Keep the MCP transport in its original library; build calls live in main.go.
import http from 'node:http';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';

http.createServer(async (req, res) => {
  const mcp = new McpServer({ name: 'xcodebuild', version: '1.0.0' });
  mcp.registerTool('build', {
    title: 'xcodebuild',
    description: 'Builds on the remote Xcode instance and installs the app on the attached simulator.',
  }, async () => {
    const response = await fetch('http://127.0.0.1:3000/xcodebuild');
    return { content: [{ type: 'text', text: await response.text() }], isError: !response.ok };
  });
  const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
  res.on('close', () => { void transport.close(); void mcp.close(); });
  try {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = Buffer.concat(chunks).toString('utf8');
    await mcp.connect(transport);
    await transport.handleRequest(req, res, body ? JSON.parse(body) : undefined);
  } catch (error) {
    if (!res.headersSent) res.writeHead(500, { 'Content-Type': 'text/plain' });
    res.end(String(error));
  }
}).listen(3001, '127.0.0.1', () => console.log('MCP endpoint: http://localhost:3001'));

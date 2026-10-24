import { http } from '@/utils/request';

// 工具接入（P1-2 OpenAPI 工具集 + P1-3 外部 MCP 服务）

export function listCustomTools() {
  return http.get('/api/custom-tools');
}

export function importCustomTool(payload) {
  return http.post('/api/custom-tools/import', payload);
}

export function deleteCustomTool(id) {
  return http.delete(`/api/custom-tools/${id}`);
}

export function listMcpServers() {
  return http.get('/api/mcp-servers');
}

export function createMcpServer(payload) {
  return http.post('/api/mcp-servers', payload);
}

export function deleteMcpServer(id) {
  return http.delete(`/api/mcp-servers/${id}`);
}

export function setMcpServerEnabled(id, enabled) {
  return http.put(`/api/mcp-servers/${id}/enabled`, { enabled });
}

export function testMcpServer(id) {
  return http.get(`/api/mcp-servers/${id}/test`);
}

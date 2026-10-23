// ack 终态文案契约：ackOutbox 默认确认 delivered；传 opts.status='failed' 时，
// v2 items[] 与 legacy msg_ids 两种请求体都必须带 failed。
// 背景：目标会话不存在（openConversation 精确切换失败）时，客户端原来只打一行日志、
// 什么都不 ack，服务端每轮领取超时后重推同一条 → 永久重试。会话真的没了就必须翻 failed。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ackOutbox } from '../src/core/http-ingest.js';

const cred = { serverUrl: 'http://localhost:8204', channel: 'douyin', accountId: 'a1', token: 't' };

function captureBodies() {
  const bodies = [];
  globalThis.fetch = vi.fn(async (_url, init) => {
    bodies.push(JSON.parse(init.body));
    return new Response(JSON.stringify({ status: 'ok' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  });
  return bodies;
}

describe('ackOutbox 终态状态', () => {
  let realFetch;
  beforeEach(() => { realFetch = globalThis.fetch; });
  afterEach(() => { globalThis.fetch = realFetch; });

  it('默认发 delivered（回归：普通成功路径不受影响）', async () => {
    const bodies = captureBodies();
    await ackOutbox(cred, ['m1'], { conversationId: 'c1' });
    expect(bodies[0].status).toBe('delivered');
    expect(bodies[0].items).toEqual([{ msg_id: 'm1', conversation_id: 'c1' }]);
  });

  it('status=failed 时 v2 items[] 请求体带 failed', async () => {
    const bodies = captureBodies();
    await ackOutbox(cred, ['m2'], { conversationId: 'c2', status: 'failed' });
    expect(bodies[0].status).toBe('failed');
    expect(bodies[0].items).toEqual([{ msg_id: 'm2', conversation_id: 'c2' }]);
  });

  it('status=failed 时 legacy 请求体同样带 failed', async () => {
    const bodies = captureBodies();
    await ackOutbox(cred, ['m3'], { status: 'failed' });
    expect(bodies[0].status).toBe('failed');
    expect(bodies[0].msg_ids).toEqual(['m3']);
  });
});

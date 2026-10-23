// 目标会话不存在时的终态处置。
//
// sendOutbound 的 notFound 来自 openConversation({exact:true}) 失败或切会话后漂移：
// 客户会话列表里没有它（被删/改名/从没存在）。旧行为只打日志、不 ack，服务端行留在
// pending，每轮领取超时重推一次，客户端每次都失败——无限重试，日志里就是"反复尝试发送"。
// 现在的契约：
//   · 页面可见时 notFound = 可信结论 ⇒ ack failed（终态，不再重推）；
//   · 页面隐藏时 notFound 可能是后台标签页没有渲染 DOM 的假阴性 ⇒ 不 ack，留给下一轮/重推。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

function makeChromeStorage() {
  const data = {};
  return {
    storage: {
      local: {
        get: vi.fn((keys) => {
          const out = {};
          const arr = Array.isArray(keys) ? keys : [keys];
          for (const k of arr) if (k in data) out[k] = data[k];
          return Promise.resolve(out);
        }),
        set: vi.fn((obj) => { Object.assign(data, obj); return Promise.resolve(); }),
      },
    },
    runtime: { sendMessage: vi.fn(async () => {}) },
    _data: data,
  };
}

let live = null;

vi.mock('../src/core/http-ingest.js', () => ({
  getOutbox: vi.fn(),
  ackOutbox: vi.fn(),
}));

vi.mock('../src/core/sanitize.js', () => ({
  sanitizeForDisplay: (t) => t,
  stripMarkdownForDM: (t) => t,
}));

vi.mock('../src/core/sse-fetch-client.js', () => ({
  connectSSE: (channel, accountId, opts) => new Promise(() => { live = { opts }; }),
  getLastEventID: () => '',
  setLastEventID: () => {},
  stopSSE: () => false,
}));

import { pollDownlink, startSSEDelivery } from '../src/core/downlink.js';
import { getOutbox, ackOutbox } from '../src/core/http-ingest.js';

const cfg = () => async () => ({ serverUrl: 'http://localhost:8204', token: 't' });

describe('notFound 终态 ack', () => {
  let st;
  beforeEach(() => {
    st = makeChromeStorage();
    globalThis.chrome = st;
    vi.clearAllMocks();
    ackOutbox.mockResolvedValue({ status: 'ok' });
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  });
  afterEach(() => {
    delete globalThis.chrome;
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  });

  it('轮询：可见页上 notFound → ack failed（带会话归属，不再留 pending）', async () => {
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-nf1', content: '发不出去', conversation_id: 'c-gone' }],
    });
    const sendOutbound = vi.fn(async () => ({ ok: false, notFound: true, rateLimited: false }));

    await pollDownlink('nf_poll', 'acc1', cfg(), { sendOutbound });

    expect(sendOutbound).toHaveBeenCalledTimes(1);
    expect(ackOutbox).toHaveBeenCalledTimes(1);
    const [, ids, opts] = ackOutbox.mock.calls[0];
    expect(ids).toEqual(['m-nf1']);
    expect(opts.status).toBe('failed');
    expect(opts.items).toEqual([{ msg_id: 'm-nf1', conversation_id: 'c-gone' }]);
  });

  it('轮询：页面隐藏时 notFound 不可信 → 不 ack（留在 pending 等下一轮）', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' });
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-nf2', content: '发不出去', conversation_id: 'c-hidden' }],
    });

    await pollDownlink('nf_hidden', 'acc1', cfg(), {
      sendOutbound: async () => ({ ok: false, notFound: true, rateLimited: false }),
    });

    expect(ackOutbox).not.toHaveBeenCalled();
  });

  it('轮询：notFound ack 失败不写已发缓存（下一轮重推时仍会再试）', async () => {
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-nf3', content: '发不出去', conversation_id: 'c-gone2' }],
    });
    ackOutbox.mockResolvedValue({ status: 'error' });

    await pollDownlink('nf_ackfail', 'acc1', cfg(), {
      sendOutbound: async () => ({ ok: false, notFound: true, rateLimited: false }),
    });

    const keys = (st._data.bridge_sent_nf_ackfail || []).map((e) => (Array.isArray(e) ? e[0] : e));
    expect(keys).not.toContain('m-nf3|c-gone2');
  });

  it('SSE：可见页上 notFound → ack failed（用 DB 会话归属，不是页面定位键）', async () => {
    const stop = await startSSEDelivery('nf_sse', 'acc1', {
      sendOutbound: async () => ({ ok: false, notFound: true, rateLimited: false }),
    });
    for (let i = 0; i < 20 && !live; i++) await new Promise((r) => setTimeout(r, 5));
    expect(live).toBeTruthy();

    await live.opts.onMessage({
      msg_id: 'm-nf4',
      conversation_id: 'c-sse-gone',
      content: '发不出去',
    });
    await stop();

    expect(ackOutbox).toHaveBeenCalledTimes(1);
    const [, ids, opts] = ackOutbox.mock.calls[0];
    expect(ids).toEqual(['m-nf4']);
    expect(opts.status).toBe('failed');
    expect(opts.items).toEqual([{ msg_id: 'm-nf4', conversation_id: 'c-sse-gone' }]);
  });
});

// 跨 tab 去重：同一个渠道/账号可能同时开着多个聊天页，每个 content script 都跑自己的
// 下行循环。SentCache 原先只在启动时 load 一次、has() 只读本 tab 内存，且 flush() 整表覆盖：
//   - A tab 发完写入缓存 → B tab 看不见，同一条消息被第二个 tab 再发一次（客户收到两遍）；
//   - 两个 tab 先后 flush → 后写的把先写的条目整表覆盖掉，去重记录丢失。
// 这里钉住三件事：发送前必须 refresh 看到了别的 tab 的写入；flush 必须合并而不是覆盖；
// 真正的发送临界区要经过跨 tab 锁（navigator.locks 可用时）。
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

vi.mock('../src/core/http-ingest.js', async (importOriginal) => {
  const actual = await importOriginal();
  return { ...actual, getOutbox: vi.fn(), ackOutbox: vi.fn() };
});
vi.mock('../src/core/sanitize.js', () => ({
  sanitizeForDisplay: (t) => t,
  stripMarkdownForDM: (t) => t,
}));

import { pollDownlink, initDownlink, withOutboundLock } from '../src/core/downlink.js';
import { getOutbox, ackOutbox } from '../src/core/http-ingest.js';

const cfg = () => async () => ({ serverUrl: 'http://localhost:8204', token: 't' });
const HOUR = 3600_000;

describe('跨 tab SentCache', () => {
  let st;
  beforeEach(() => { st = makeChromeStorage(); globalThis.chrome = st; vi.clearAllMocks(); });
  afterEach(() => { delete globalThis.chrome; delete globalThis.navigator.locks; });

  it('另一个 tab 已写缓存（本 tab 加载后才写入）→ 本 tab 不再发送，只补 ack', async () => {
    await initDownlink(['xtab_a']); // 本 tab 先加载，此时存储为空
    // 另一个 tab 发完并落盘
    st._data.bridge_sent_xtab_a = [[`m-xt|c1`, Date.now()]];
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-xt', content: '已经发过', conversation_id: 'c1' }],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    const sendOutbound = vi.fn(async () => ({ ok: true }));

    await pollDownlink('xtab_a', 'acc1', cfg(), { sendOutbound });

    expect(sendOutbound).not.toHaveBeenCalled();
    expect(ackOutbox).toHaveBeenCalledTimes(1);
    expect(ackOutbox.mock.calls[0][1]).toEqual(['m-xt']);
  });

  it('flush 是合并写：另一个 tab 的条目不会被本 tab 的整表覆盖抹掉', async () => {
    await initDownlink(['xtab_b']);
    st._data.bridge_sent_xtab_b = [[`m-other|c9`, Date.now()]];
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-mine', content: '本 tab 新发', conversation_id: 'c1' }],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xtab_b', 'acc1', cfg(), { sendOutbound: async () => ({ ok: true }) });

    const keys = (st._data.bridge_sent_xtab_b || []).map((e) => (Array.isArray(e) ? e[0] : e));
    expect(keys).toContain('m-mine|c1');
    expect(keys).toContain('m-other|c9'); // 被覆盖丢掉 = 下轮会重发别人发过的消息
  });

  it('发送临界区串行：两个并发调用不重叠执行（navigator.locks 语义）', async () => {
    const held = new Set();
    const queued = new Map();
    globalThis.navigator = globalThis.navigator || {};
    globalThis.navigator.locks = {
      request(name, fn) {
        if (held.has(name)) {
          return new Promise((resolve) => {
            if (!queued.has(name)) queued.set(name, []);
            queued.get(name).push(() => resolve(fn()));
          });
        }
        held.add(name);
        return Promise.resolve(fn()).finally(() => {
          held.delete(name);
          const q = queued.get(name);
          if (q && q.length) q.shift()();
        });
      },
    };

    const order = [];
    let active = 0;
    const run = () => withOutboundLock('same-key', async () => {
      active++;
      expect(active).toBe(1); // 进入临界区时不许有另一个持锁者
      order.push('start');
      await new Promise((r) => setTimeout(r, 20));
      order.push('end');
      active--;
    });
    await Promise.all([run(), run()]);
    expect(order).toEqual(['start', 'end', 'start', 'end']);
  });

  it('navigator.locks 不可用时退化为直接执行（不阻塞下发）', async () => {
    delete globalThis.navigator.locks;
    const fn = vi.fn(async () => 'ok');
    await expect(withOutboundLock('any', fn)).resolves.toBe('ok');
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('轮询发送会请求跨 tab 锁（不是只刷新缓存就算数）', async () => {
    const requested = [];
    globalThis.navigator = globalThis.navigator || {};
    globalThis.navigator.locks = {
      request(name, fn) { requested.push(name); return Promise.resolve(fn()); },
    };
    await initDownlink(['xtab_c']);
    getOutbox.mockResolvedValue({
      status: 'ok',
      messages: [{ msg_id: 'm-lock', content: '锁', conversation_id: 'c1' }],
    });
    ackOutbox.mockResolvedValue({ status: 'ok' });
    await pollDownlink('xtab_c', 'acc1', cfg(), { sendOutbound: async () => ({ ok: true }) });
    expect(requested.some((n) => n.includes('m-lock|c1'))).toBe(true);
  });
});

// 让 HOUR 参与编译期引用，避免 lint 把未使用常量当成噪音（保留给未来的时间界用例）。
void HOUR;

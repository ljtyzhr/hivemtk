// 批25：换会话互斥 —— 不可逆性优先级：下发 > 巡检。
//
// 病灶（读码取证）：巡检（patrol/_patrolVisit）与下发（sendOutbound）都走
// openConversation 点左栏换右侧会话，两条链路互不知情。下发转发整条链路都是扩展自己实现的
// （轮询 outbox → 切会话 → 逐段拟人 insertText → 点发送 → send_verified 回查 → 自报上行），
// 键入没有人工参与：30 字≈2.7s、100 字≈9s，而巡检切换节拍 3–5s ⇒ 切换节拍 < 键入窗，
// 键入期被切走是结构性必现。后果：A 的文案被点进切换后的 B 会话 ——「正在给 A 打字，
// 跳到了下一个用户」，不可撤回；而 send_verified 回查在错误的会话里数气泡只会判"未见"，
// ack 侧照样记成功。
//
// 本批契约（单向，无相互等待 ⇒ 无死锁）：
//   ① 下发从 openConversation 起到「键入 + 点击 + 回查 + 自报上行入队」全程持 _outboundInFlight，
//     期间巡检/全量同步不得换会话；释放后只剩纯 HTTP，不碰 DOM；
//   ② 释放后还有 outboundLingerMs 的 linger 尾巴：downlink 是逐条 sendOutbound 的，两条消息
//     之间有 await 空档，不盖住巡检就会插进来切一次会话、下一条再切回去（白跑两次切换）；
//   ③ 挡不住的情形（SPA 自己改路由 / 用户手点）由"点发送前复核"兜底：漂移 ⇒ 清空输入框
//     + 抛 conv-drift ⇒ 本条不 ack、留 pending 重投；
//   ④ 对称方向：巡检采集前复核"还停在 openConversation 打开的那个会话"，不一致就丢弃
//     —— 否则别的会话的消息会被挂在它名下上行（串台）。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { assertConvOrAbort } from '../src/core/dom.js';
import { SENDER, CHANNELS } from '../src/core/types.js';
import { BRIDGE_THREE_CHANNEL } from '../src/core/constants.js';

beforeEach(() => {
  localStorage.clear();
  document.body.innerHTML = '';
});

afterEach(() => {
  document.body.innerHTML = '';
});

function makeVisible(el) {
  Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
  return el;
}

function makeConv(id, name) {
  const el = makeVisible(document.createElement('button'));
  el.setAttribute('data-e2e', 'conversation-item');
  return { id, name, el };
}

// 巡检参数：等窗口的节拍压到毫秒级，用例才不会真的去等默认的 20s 上界。
const FAST_PATROL = {
  throttleMs: 1, waitActiveMs: 100, switchMinMs: 0, switchMaxMs: 0,
  switchBlockWaitMs: 300, switchBlockPollMs: 20,
};

/**
 * 会话切换骨架：点会话项 ⇒ 活动会话变为该会话。
 */
function buildConvAdapter(convList) {
  let current = convList[0] ? convList[0].id : null;
  const hooks = {
    match: () => true,
    getAccountId: () => 'acc25',
    getConversationId: () => current,
    getMessageRoot: () => null,
    getMessageItems: () => [],
    parseMessageItem: () => null,
    getConversationList: () => convList,
  };
  const adapter = new BaseAdapter({ name: 'b25', channel: CHANNELS.DOUYIN, SEL: {}, hooks });
  convList.forEach((conv) => {
    conv.el.addEventListener('click', () => { current = conv.id; });
  });
  return { adapter, getCurrent: () => current };
}

describe('批25-①②：下发在途（含 linger）⇒ 巡检不换会话', () => {
  it('下发持锁期间换会话一律让路：openConversation 返回 null、活动会话不动', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter, getCurrent } = buildConvAdapter([A, B]);

    adapter.beginOutbound();
    const opened = await adapter.openConversation('b2', { waitActiveMs: 200 });
    expect(opened).toBeNull();
    expect(getCurrent()).toBe('a1');
    expect(adapter.patrolStatus().yielded.sendInFlight).toBe(1);

    adapter.endOutbound();
    adapter._outboundLingerUntil = 0;   // linger 由下一条用例单独验，这里手动抹掉
    const opened2 = await adapter.openConversation('b2', { waitActiveMs: 200 });
    expect(opened2).toBe('b2');
  });

  it('巡检一轮在下发在途时整轮让路：visited=0、当前会话不变、返回 yielded 判据', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter, getCurrent } = buildConvAdapter([A, B]);
    const onMessage = vi.fn();
    adapter.callbacks.onMessage = onMessage;

    adapter.beginOutbound();
    const r = await adapter.patrol(FAST_PATROL);
    adapter.endOutbound();

    expect(r.visited).toBe(0);
    expect(r.yielded).toBe('send-in-flight');
    expect(r.skippedByYield).toBe(1); // a1 是当前会话被跳过，只有 b2 在等窗口
    expect(getCurrent()).toBe('a1');
    expect(onMessage).not.toHaveBeenCalled();
  });

  it('endOutbound 后 linger 未过仍算忙：isSendBusy()=true、换会话继续让路、巡检整轮让路', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter, getCurrent } = buildConvAdapter([A, B]);

    adapter.beginOutbound();
    adapter.endOutbound();
    // 临界区已释放，但 linger 尾巴还在 —— 这段时间正是 downlink 两条消息之间的 await 空档
    expect(adapter.isSendBusy()).toBe(true);
    expect(await adapter.openConversation('b2', { waitActiveMs: 200 })).toBeNull();
    expect(getCurrent()).toBe('a1');

    const r = await adapter.patrol(FAST_PATROL);
    expect(r.visited).toBe(0);
    expect(r.yielded).toBe('send-in-flight');
    expect(adapter.patrolStatus().sendBusy).toBe(true);
    expect(getCurrent()).toBe('a1');

    // linger 过期（等价于时间走过 outboundLingerMs）⇒ 立刻恢复，不必等下一轮
    adapter._outboundLingerUntil = Date.now() - 1;
    expect(adapter.isSendBusy()).toBe(false);
    expect(adapter.patrolStatus().sendBusy).toBe(false);
    expect(await adapter.openConversation('b2', { waitActiveMs: 200 })).toBe('b2');
  });

  it('下发短暂持锁（含 linger）释放后，巡检在同一轮里等到窗口就继续，不整轮作废', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter, getCurrent } = buildConvAdapter([A, B]);
    const switches = [];
    for (const c of [A, B]) c.el.addEventListener('click', () => switches.push(c.id));

    // 模拟一次真实下发：巡检开跑后 ~120ms 才释放，释放后还要走完 linger 尾巴才算不忙
    adapter.beginOutbound();
    setTimeout(() => adapter.endOutbound(), 120);
    const t0 = Date.now();

    const r = await adapter.patrol({ ...FAST_PATROL, switchBlockWaitMs: 5000 });
    const elapsed = Date.now() - t0;

    expect(r.yielded).toBeNull();          // 等到了窗口 ⇒ 本轮没有作废
    expect(r.skippedByYield).toBe(0);
    expect(r.visited).toBe(1);             // b2 被访问
    expect(switches).toEqual(['b2', 'a1']); // 访问 b2，收尾回切巡检前的 a1
    expect(getCurrent()).toBe('a1');
    // linger 盖住了两条下发之间的空档：巡检必须等尾巴过去才动手
    expect(elapsed).toBeGreaterThanOrEqual(BRIDGE_THREE_CHANNEL.outboundLingerMs);
  }, 15000);
});

describe('批25-③：点发送前复核会话归属（最后一道闸）', () => {
  it('键入期会话漂移 ⇒ 清空输入框 + 抛 conv-drift（绝不点发送）', () => {
    const box = document.createElement('div');
    box.setAttribute('contenteditable', 'true');
    box.textContent = '给 A 的回复';

    expect(() => assertConvOrAbort(box, () => false)).toThrowError(/conv-drift/);
    expect(box.textContent).toBe('');   // 半截草稿不许留给下一个用户
  });

  it('未漂移 ⇒ 原样放行（不碰输入框）', () => {
    const box = document.createElement('div');
    box.setAttribute('contenteditable', 'true');
    box.textContent = '给 A 的回复';
    expect(() => assertConvOrAbort(box, () => true)).not.toThrow();
    expect(box.textContent).toBe('给 A 的回复');
  });

  it('sendOutbound 把 assertConv 传给渠道 sendText：漂移 ⇒ ok=false、drifted=true、无 AGENT 回声帧', async () => {
    const box = document.createElement('div');
    box.setAttribute('contenteditable', 'true');
    document.body.appendChild(box);
    let current = 'conv-a';
    const adapter = new BaseAdapter({
      name: 'b25-send',
      channel: CHANNELS.DOUYIN,
      SEL: {},
      hooks: {
        match: () => true,
        getAccountId: () => 'acc25',
        getConversationId: () => current,
        getMessageItems: () => [],
        getMessageRoot: () => null,
        parseMessageItem: () => null,
        // 模拟"键入到一半 SPA 把活动会话挪走"：填完字、点之前漂移
        sendText: async (text, opts = {}) => {
          box.textContent = text;
          current = 'conv-b';
          assertConvOrAbort(box, opts.assertConv);
        },
      },
    });
    const onMessage = vi.fn();
    expect(adapter.start({ onMessage })).toBe(true);

    const res = await adapter.sendOutbound('给 A 的回复', undefined, { sendVerifyMs: 1 });

    expect(res.ok).toBe(false);
    expect(res.drifted).toBe(true);
    expect(res.notFound).toBe(false);          // 不能走"目标不存在"那条 failed ack 出路
    expect(box.textContent).toBe('');          // 输入框已清空
    expect(onMessage).not.toHaveBeenCalled();  // 没有"已发出"的假回声
    adapter.stop();
  });

  it('sendOutbound 全程持锁：键入期间 isSendBusy()=true，结束后只剩 linger 尾巴，过期即放锁（否则巡检永久让路）', async () => {
    let busyDuringTyping = null;
    const adapter = new BaseAdapter({
      name: 'b25-lock',
      channel: CHANNELS.DOUYIN,
      SEL: {},
      hooks: {
        match: () => true,
        getAccountId: () => 'acc25',
        getConversationId: () => 'conv-a',
        getMessageItems: () => [],
        getMessageRoot: () => null,
        parseMessageItem: () => null,
        sendText: async () => { busyDuringTyping = adapter.isSendBusy(); },
      },
    });
    expect(adapter.start({ onMessage: vi.fn() })).toBe(true);
    const res = await adapter.sendOutbound('在的', undefined, { sendVerifyMs: 1 });
    expect(busyDuringTyping).toBe(true);
    // 临界区已释放，但 linger 尾巴还在（盖住 downlink 两条消息之间的 await 空档）
    expect(adapter.isSendBusy()).toBe(true);
    adapter._outboundLingerUntil = Date.now() - 1;   // 尾巴过期 ⇒ 必须真放锁
    expect(adapter.isSendBusy()).toBe(false);
    expect(res.ok).toBe(true);
    adapter.stop();
  });
});

describe('批25-④：对称方向 —— 巡检采集前复核归属，漂移即丢弃', () => {
  it('_patrolVisit 打开会话后、采集前被切走 ⇒ 不上行（别把 B 的消息挂在 A 名下）', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    let current = 'a1';
    const onMessage = vi.fn();
    const adapter = new BaseAdapter({
      name: 'b25-patrol',
      channel: CHANNELS.DOUYIN,
      SEL: {},
      hooks: {
        match: () => true,
        getAccountId: () => 'acc25',
        getConversationId: () => current,
        getMessageRoot: () => null,
        getMessageItems: () => {
          const el = makeVisible(document.createElement('div'));
          el.textContent = current === 'b2' ? '李四说的话' : '张三说的话';
          return [el];
        },
        parseMessageItem: (item) => ({ sender_type: SENDER.CUSTOMER, text: item.textContent, timestamp: 1 }),
        getConversationList: () => [A, B],
      },
    });
    adapter.callbacks.onMessage = onMessage;

    // 打开 b2（此时 current=b2），然后在这之后的 renderWait 窗口里被"别人"切回 a1
    const realOpen = adapter.openConversation.bind(adapter);
    vi.spyOn(adapter, 'openConversation').mockImplementation(async (cid, opts) => {
      const r = await realOpen(cid, opts);
      setTimeout(() => { current = 'a1'; }, 0);
      return r;
    });
    // openConversation 被让路判据拦下时会直接返回 null，这里不涉及：无下发在途、linger 也已过
    B.el.addEventListener('click', () => { current = 'b2'; });

    const r = await adapter._patrolVisit(B, { throttleMs: 600, waitActiveMs: 400 });

    expect(r.ok).toBe(false);
    expect(r.drifted).toBe(true);
    expect(onMessage).not.toHaveBeenCalled();
    vi.restoreAllMocks();
  });
});

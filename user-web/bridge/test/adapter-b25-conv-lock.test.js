// 批25：换会话互斥 —— 不可逆性优先级：人工键入 > 下发 > 巡检。
//
// 病灶（读码取证）：巡检（patrol/_patrolVisit）与下发（sendOutbound）都走
// openConversation 点左栏换右侧会话，两条链路互不知情。下发的键入是逐段拟人 insertText
// （30 字≈2.7s、100 字≈9s），巡检切换节拍 3–5s ⇒ 切换节拍 < 键入窗，键入期被切走是
// 结构性必现。后果：A 的文案被点进切换后的 B 会话 ——「正在给 A 打字，跳到了下一个用户」，
// 不可撤回；而 send_verified 回查在错误的会话里数气泡只会判"未见"，ack 侧照样记成功。
//
// 本批契约（单向，无相互等待 ⇒ 无死锁）：
//   ① 下发全程持 _outboundInFlight，期间巡检/全量同步不得换会话；
//   ② 人工正在输入（近期键入 or 输入框有草稿）时巡检也不得换会话；
//   ③ 挡不住的情形（SPA 自己改路由 / 用户手点）由"点发送前复核"兜底：漂移 ⇒ 清空输入框
//     + 抛 conv-drift ⇒ 本条不 ack、留 pending 重投；
//   ④ 对称方向：巡检采集前复核"还停在 openConversation 打开的那个会话"，不一致就丢弃
//     —— 否则别的会话的消息会被挂在它名下上行（串台）。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { assertConvOrAbort } from '../src/core/dom.js';
import { SENDER, CHANNELS } from '../src/core/types.js';

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
function buildConvAdapter(convList, { inputText } = {}) {
  let current = convList[0] ? convList[0].id : null;
  const hooks = {
    match: () => true,
    getAccountId: () => 'acc25',
    getConversationId: () => current,
    getMessageRoot: () => null,
    getMessageItems: () => [],
    parseMessageItem: () => null,
    getConversationList: () => convList,
    // 输入框草稿由用例直接给（不打 DOM 选择器的擂台）
    getInputDraftText: () => (typeof inputText === 'function' ? inputText() : (inputText || '')),
  };
  const adapter = new BaseAdapter({ name: 'b25', channel: CHANNELS.DOUYIN, SEL: {}, hooks });
  convList.forEach((conv) => {
    conv.el.addEventListener('click', () => { current = conv.id; });
  });
  return { adapter, getCurrent: () => current };
}

describe('批25-①②：下发在途 / 人工输入中 ⇒ 巡检不换会话', () => {
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

  it('人工正在输入（输入框有草稿）⇒ 巡检同样让路；草稿清空后下一轮恢复', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    let draft = '我正在手打一段话';
    const { adapter, getCurrent } = buildConvAdapter([A, B], { inputText: () => draft });
    const switches = [];
    for (const c of [A, B]) c.el.addEventListener('click', () => switches.push(c.id));

    const r1 = await adapter.patrol(FAST_PATROL);
    expect(r1.visited).toBe(0);
    expect(r1.yielded).toBe('human-typing');
    expect(adapter.patrolStatus().yielded.humanTyping).toBe(1);
    expect(getCurrent()).toBe('a1');
    expect(switches).toEqual([]);   // 一次会话都没点

    draft = '';
    const r2 = await adapter.patrol(FAST_PATROL);
    expect(r2.visited).toBe(1);
    expect(r2.yielded).toBeNull();
    expect(switches).toEqual(['b2', 'a1']);  // 访问过 b2，收尾回切到巡检前的 a1
  });

  it('草稿停留超过上界后不再让路（防一条被放弃的残留草稿把巡检永久按停）', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter } = buildConvAdapter([A, B], { inputText: () => '被放弃的残留草稿' });

    expect(adapter.isHumanTyping()).toBe(true);       // 首次看到 ⇒ 让路
    const r1 = await adapter.patrol(FAST_PATROL);
    expect(r1.yielded).toBe('human-typing');

    adapter._draftFirstSeenAt = Date.now() - 10 * 60 * 1000;  // 越过 draftStaleMs
    const r2 = await adapter.patrol(FAST_PATROL);
    expect(r2.yielded).toBeNull();
    expect(r2.visited).toBe(1);
  });

  it('等窗口有上界：人工一直输入时巡检不会无限期挂住，到点即放弃本轮', async () => {
    const A = makeConv('a1', '张三');
    const B = makeConv('b2', '李四');
    const { adapter } = buildConvAdapter([A, B], { inputText: () => '一直在打字' });
    const t0 = Date.now();
    const r = await adapter.patrol({ ...FAST_PATROL, switchBlockWaitMs: 1200 });
    const elapsed = Date.now() - t0;
    expect(r.yielded).toBe('human-typing');
    expect(elapsed).toBeGreaterThanOrEqual(1000);
    expect(elapsed).toBeLessThan(5000);
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

  it('sendOutbound 全程持锁：键入期间 isSendBusy()=true，结束后必须放锁（否则巡检永久让路）', async () => {
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
    // openConversation 被让路判据拦下时会直接返回 null，这里不涉及：无锁无草稿
    B.el.addEventListener('click', () => { current = 'b2'; });

    const r = await adapter._patrolVisit(B, { throttleMs: 600, waitActiveMs: 400 });

    expect(r.ok).toBe(false);
    expect(r.drifted).toBe(true);
    expect(onMessage).not.toHaveBeenCalled();
    vi.restoreAllMocks();
  });
});

import { describe, it, expect, beforeEach } from 'vitest';
import {
  locateMessages,
  expandMultiBubbleItems,
  isOversizedBubble,
  BUBBLE_SPLIT_MIN_RUNES,
  BUBBLE_HARD_MAX_RUNES,
} from '../src/core/selector-engine.js';

beforeEach(() => {
  document.body.innerHTML = '';
});

// 生成一段足够长的消息正文，用来把容器推过「可能是线程容器」的长度线。
const longText = (ch, n = Math.ceil(BUBBLE_SPLIT_MIN_RUNES * 0.6)) => ch.repeat(n);

describe('expandMultiBubbleItems 拆分线程容器', () => {
  it('超长容器（内含多条气泡）拆成多个气泡项，保持 DOM 顺序', () => {
    document.body.innerHTML = `
      <div id="thread">
        <div class="row"><span>${longText('甲')}</span></div>
        <div class="row"><span>${longText('乙')}</span></div>
      </div>`;
    const thread = document.getElementById('thread');
    const rows = Array.from(thread.children);
    const out = expandMultiBubbleItems([thread]);
    expect(out.length).toBe(2);
    expect(out[0]).toBe(rows[0]);
    expect(out[1]).toBe(rows[1]);
  });

  it('单条长气泡（唯一的嵌套链）不被拆成碎片', () => {
    const text = longText('丙', BUBBLE_SPLIT_MIN_RUNES + 50);
    document.body.innerHTML = `<div id="bubble"><div class="wrap"><span>${text}</span></div></div>`;
    const bubble = document.getElementById('bubble');
    const out = expandMultiBubbleItems([bubble]);
    expect(out).toEqual([bubble]);
  });

  it('普通长度的气泡原样返回', () => {
    document.body.innerHTML = '<div id="b"><span>你好，在吗</span></div>';
    const b = document.getElementById('b');
    expect(expandMultiBubbleItems([b])).toEqual([b]);
  });
});

describe('locateMessages 与长度护栏', () => {
  it('命中超长线程容器的选择器时，返回拆分后的气泡而不是整段线程', () => {
    document.body.innerHTML = `
      <div class="message-list">
        <div data-e2e="msg-item-content">
          <div class="bubble"><span>${longText('甲')}</span></div>
          <div class="bubble"><span>${longText('乙')}</span></div>
        </div>
      </div>`;
    const { items } = locateMessages({
      root: document,
      itemSelectors: ['[data-e2e="msg-item-content"]'],
      listSelectors: ['.message-list'],
    });
    expect(items.length).toBe(2);
    expect(items.every((el) => el.classList.contains('bubble'))).toBe(true);
  });

  it('超过硬上限的无法再拆分的项被判定为不可信气泡', () => {
    document.body.innerHTML = `<div id="huge">${longText('丁', BUBBLE_HARD_MAX_RUNES + 1)}</div><div id="small">短消息</div>`;
    expect(isOversizedBubble(document.getElementById('huge'))).toBe(true);
    expect(isOversizedBubble(document.getElementById('small'))).toBe(false);
  });
});

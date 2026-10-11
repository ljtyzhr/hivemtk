export const CARD_PLATFORM = Object.freeze({
  DOUYIN: 'douyin',
  KUAISHOU: 'kuaishou',
  XIAOHONGSHU: 'xiaohongshu',
  XIANYU: 'xianyu',
  TIKTOK: 'tiktok'
});

export const CARD_PLATFORM_OPTIONS = Object.freeze([
  { value: 'douyin',       label: '抖音',   tagType: 'danger',        sort: 1, group: 'card',   icon: 'VideoCamera' },
  { value: 'kuaishou',     label: '快手',   tagType: 'warning',        sort: 2, group: 'card',   icon: 'VideoCamera' },
  { value: 'xiaohongshu',  label: '小红书', tagType: 'danger',  sort: 3, group: 'card',   icon: 'Postcard' },
  { value: 'xianyu',       label: '闲鱼',   tagType: 'warning', sort: 4, group: 'card',   icon: 'Goods' },
  { value: 'tiktok',       label: 'TikTok', tagType: 'danger',        sort: 5, group: 'card',   icon: 'VideoCamera' }
]);

// 线索类型的唯一事实源是 user-server/internal/service/clue.go 的 defaultClueTypes
// （接口 GET /api/clues/type 原样返回它）。这份表必须与它逐条一致，
// tests/unit/clue_type_labels_sync.test.js 每次跑都从那个 .go 文件现读一遍对账。
//
// 原来这里只有 4 条，且与后端语义错位（写的是 1=小红书/2=视频号/3=抖音/4=快手，
// 后端 1=QQ/2=微信/3=电话/4=Telegram）：线索列表的"线索类型"筛选项只有 4 个，
// 选「抖音」实际筛的是 type=3=电话；库里 67% 的线索（type 9~14）在统计页直接印成数字。
export const CLUE_TYPE_OPTIONS = Object.freeze([
  { value: '1', label: 'QQ', tagType: 'info', sort: 1, group: 'clue' },
  { value: '2', label: '微信', tagType: 'success', sort: 2, group: 'clue' },
  { value: '3', label: '电话', tagType: 'warning', sort: 3, group: 'clue' },
  { value: '4', label: 'Telegram', tagType: 'primary', sort: 4, group: 'clue' },
  { value: '5', label: 'WhatsApp', tagType: 'success', sort: 5, group: 'clue' },
  { value: '6', label: 'Twitter/X', tagType: 'info', sort: 6, group: 'clue' },
  { value: '7', label: '企业微信', tagType: 'success', sort: 7, group: 'clue' },
  { value: '8', label: '智能线索发掘', tagType: 'primary', sort: 8, group: 'clue' },
  { value: '9', label: '抖音', tagType: 'danger', sort: 9, group: 'clue' },
  { value: '10', label: '快手', tagType: 'warning', sort: 10, group: 'clue' },
  { value: '11', label: '小红书', tagType: 'danger', sort: 11, group: 'clue' },
  { value: '12', label: '闲鱼', tagType: 'warning', sort: 12, group: 'clue' },
  { value: '13', label: '飞书', tagType: 'success', sort: 13, group: 'clue' },
  { value: '14', label: 'TikTok', tagType: 'danger', sort: 14, group: 'clue' },
  { value: '15', label: '网页组件', tagType: 'primary', sort: 15, group: 'clue' },
  { value: '16', label: '邮件', tagType: 'info', sort: 16, group: 'clue' },
  { value: '17', label: '短信', tagType: 'info', sort: 17, group: 'clue' },
  { value: '99', label: '自定义', tagType: 'info', sort: 18, group: 'clue' }
]);

// 历史 6 档：仅 utils/map.js 的旧接口还在引，标签一律以 CLUE_TYPE_OPTIONS 为准
export const CLUE_TYPE_OPTIONS_LEGACY = Object.freeze([
  { value: '1', label: 'QQ',         tagType: 'info', sort: 1, group: 'legacy' },
  { value: '2', label: '微信',       tagType: 'success', sort: 2, group: 'legacy' },
  { value: '3', label: '电话',       tagType: 'warning', sort: 3, group: 'legacy' },
  { value: '4', label: 'Telegram',   tagType: 'primary', sort: 4, group: 'legacy' },
  { value: '5', label: 'Whatsapp',   tagType: 'success', sort: 5, group: 'legacy' },
  { value: '6', label: 'twitter',    tagType: 'info', sort: 6, group: 'legacy' }
]);

const _buildLabelMap = (arr) => arr.reduce((acc, o) => {
  acc[o.value] = o.label
  acc[String(o.value)] = o.label
  return acc
}, {});
const _buildTagTypeMap = (arr) => arr.reduce((acc, o) => {
  acc[o.value] = o.tagType
  acc[String(o.value)] = o.tagType
  return acc
}, {})

export const CARD_PLATFORM_LABEL_MAP = Object.freeze(_buildLabelMap(CARD_PLATFORM_OPTIONS))
export const CARD_PLATFORM_TAG_TYPE_MAP = Object.freeze(_buildTagTypeMap(CARD_PLATFORM_OPTIONS))
// 只从权威表生成：早先是 {...OPTIONS, ...LEGACY}，LEGACY 把 1~6 又盖了一遍
// （WhatsApp→Whatsapp、Twitter/X→twitter），两处的"哪个是正式写法"就此说不清。
export const CLUE_TYPE_LABEL_MAP = Object.freeze(_buildLabelMap(CLUE_TYPE_OPTIONS))

export const getCardPlatformLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return CARD_PLATFORM_LABEL_MAP[value]
    || CLUE_TYPE_LABEL_MAP[value]
    || String(value)
};

export const getCardPlatformTagType = (value) => {
  if (value === undefined || value === null || value === '') return ''
  return CARD_PLATFORM_TAG_TYPE_MAP[value] || CLUE_TYPE_LABEL_MAP[value] || 'info'
};

export const getClueTypeLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  // 兜底要读出"这是一档没登记过的类型"，而不是一枚裸数字——裸数字看起来像页面渲染坏了
  return CLUE_TYPE_LABEL_MAP[value] || CARD_PLATFORM_LABEL_MAP[value] || `未知类型 ${value}`
};

export const getClueTypeOptions = () => CLUE_TYPE_OPTIONS;

export const filterCardPlatformByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return [
    ...CARD_PLATFORM_OPTIONS,
    ...CLUE_TYPE_OPTIONS,
    ...CLUE_TYPE_OPTIONS_LEGACY
  ].filter((o) => list.includes(o.group))
};

export default {
  CARD_PLATFORM,
  CARD_PLATFORM_OPTIONS,
  CLUE_TYPE_OPTIONS,
  CLUE_TYPE_OPTIONS_LEGACY,
  CARD_PLATFORM_LABEL_MAP,
  CARD_PLATFORM_TAG_TYPE_MAP,
  CLUE_TYPE_LABEL_MAP,
  getCardPlatformLabel,
  getCardPlatformTagType,
  getClueTypeLabel,
  getClueTypeOptions,
  filterCardPlatformByGroup
}

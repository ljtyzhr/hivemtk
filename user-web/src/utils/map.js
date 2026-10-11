import { getClueTypeOptions, CLUE_TYPE_LABEL_MAP } from '@/constants/cardPlatform';
import { getOrderStatusLabel, getOrderStatusTagType } from '@/constants/orderStatus'

// 档位名称/颜色只认 constants/cardPlatform 的权威表（与后端 defaultClueTypes 对账）。
// 这里曾另存三份互不相同的抄本：1=小红书/2=视频号/3=抖音/4=快手 的名称表、同键不同色的
// 标签表、以及只覆盖 6 档的 getClueMap/getClueName —— 后端 1=QQ/2=微信/3=电话/4=Telegram，
// 全部错位。本模块目前没有生产调用方（只有它自己的单测引用），但留着就还会被新页面接错。
const _findOption = (type) =>
  getClueTypeOptions().find((o) => String(o.value) === String(type))

export const getPlatformName = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return CLUE_TYPE_LABEL_MAP[String(type)] ?? '未知'
};

export const getPlatformTag = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return _findOption(type)?.tagType ?? '未知'
};

export const getPlatformMap = () => getClueTypeOptions();

const _LEGACY_STATUS_TYPE_MAP = {
  '1': 'info',
  '5': 'warning',
  '8': 'success',
  '9': 'info',
  '10': 'danger'
};
const _LEGACY_STATUS_NAME_MAP = {
  '1': '待执行',
  '5': '进行中',
  '8': '已失败',
  '9': '已取消',
  '10': '已完成'
}

export const getStatusTag = (status) => _LEGACY_STATUS_TYPE_MAP[status] || getOrderStatusTagType(status) || 'info';

export const getStatusName = (status) => _LEGACY_STATUS_NAME_MAP[status] || getOrderStatusLabel(status) || '未知';

export const getClueMap = () => getClueTypeOptions().slice();

export const getClueName = getPlatformName;

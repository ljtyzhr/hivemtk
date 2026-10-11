// 一条错误值不值得占用控制台的行数，取决于「用户是否已经被告知」。
//
// request.js 的拦截器在 reject 之前已经把后端的文案弹给了用户（或调用方显式声明了
// _silent，改由页面自己的常驻告警位说明），此时组件的事件处理器没接住这条拒绝，
// 补出来的只是同一个失败的第二次记账——而控制台一旦常年被这种重复占满，真故障就会被淹掉。
//
// 另一类根本不是失败：Element Plus 的 ElMessageBox 用 reject('cancel'|'close') 表达
// 「用户打开了确认框又改了主意」，这是正常控制流。走查里点取消的人数远多于碰服务端故障的人。
//
// 标记用字符串键 + 非枚举属性，两处都是必需的：
//   - 非枚举：不随 JSON.stringify 进日志，也不出现在 Object.keys 里；
//   - 字符串而非 Symbol：模块被重新实例化时（vitest 的 vi.resetModules、Vite 的 HMR）
//     Symbol 会换成新的一枚，旧错误上的标记就再也认不出来——放过的那两类会当场变成"没放过"。
const REPORTED_KEY = '__userReported__'

export function markUserReported(error) {
  if (!error || (typeof error !== 'object' && typeof error !== 'function')) return error
  try {
    Object.defineProperty(error, REPORTED_KEY, { value: true, enumerable: false, configurable: true, writable: true })
  } catch { /* 冻结过的错误对象：标不上就按"未报"处理，兜底照打，不放走任何未证实的东西 */ }
  return error
}

export function isUserReportedError(reason) {
  if (reason === 'cancel' || reason === 'close') return true
  if (!reason || (typeof reason !== 'object' && typeof reason !== 'function')) return false
  return reason[REPORTED_KEY] === true
}

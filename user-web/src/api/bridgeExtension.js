import { http } from '@/utils/request'

// 桥接扩展安装包（不上架 Chrome Web Store，平台运营上传、租户在此下载）。
//
// _silent：404 在这里是**设计内状态**而不是故障 —— 后端契约（controller/
// bridge_extension.go）明确：200=有包，404=当前部署未接入平台或平台尚未上架，
// 503=平台侧调用失败。页面（views/bridge/ExtensionPackage.vue）已经把这三种
// 状态分别渲染成常驻告警位，不带 _silent 的话拦截器还会再弹一条「请求地址不
// 存在」—— 404 在这里的真实含义是「现在拿不到包」，不是「地址写错了」，
// 两条信号打架还会把页面那句真话盖掉。
export const getBridgeExtensionPackage = () => http.get('/api/bridge/extension/package', undefined, { _silent: true })

import { http } from '@/utils/request'

// 桥接扩展安装包（不上架 Chrome Web Store，平台运营上传、租户在此下载）。
export const getBridgeExtensionPackage = () => http.get('/api/bridge/extension/package')

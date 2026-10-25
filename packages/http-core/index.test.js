// @hivemtk/http-core 单测（验收：抽包后行为不变，bridge/platform-web
// 迁移时以本套用例为契约）。零依赖，node --test 或 vitest 均可跑。
import { describe, it, expect } from 'vitest'
import {
  BIZ_CODE_MODULE_NOT_ENABLED,
  BIZ_CODE_SUBSCRIPTION_EXPIRED,
  isBizSuccess,
  isJsonContentType,
  extractServerMessage,
  createBizError
} from './index.js'

describe('isBizSuccess', () => {
  it('识别三种成功拼写', () => {
    expect(isBizSuccess('SUCCESS')).toBe(true)
    expect(isBizSuccess(200)).toBe(true)
    expect(isBizSuccess(0)).toBe(true)
  })
  it('非成功码不误判', () => {
    expect(isBizSuccess(4301)).toBe(false)
    expect(isBizSuccess(4305)).toBe(false)
    expect(isBizSuccess(undefined)).toBe(false)
    expect(isBizSuccess('success')).toBe(false) // 大小写敏感：服务端约定是全大写
  })
})

describe('isJsonContentType', () => {
  it('application/json 命中（大小写不敏感）', () => {
    expect(isJsonContentType({ 'content-type': 'application/json; charset=utf-8' })).toBe(true)
    expect(isJsonContentType({ 'Content-Type': 'Application/JSON' })).toBe(true)
  })
  it('非 JSON 与空 headers 不命中', () => {
    expect(isJsonContentType({ 'content-type': 'text/html' })).toBe(false)
    expect(isJsonContentType(null)).toBe(false)
    expect(isJsonContentType({})).toBe(false)
  })
})

describe('extractServerMessage', () => {
  it('message 优先于 msg，缺失回落 fallback', () => {
    expect(extractServerMessage({ message: 'a', msg: 'b' }, 'fb')).toBe('a')
    expect(extractServerMessage({ msg: 'b' }, 'fb')).toBe('b')
    expect(extractServerMessage({}, 'fb')).toBe('fb')
    expect(extractServerMessage(null, 'fb')).toBe('fb')
    expect(extractServerMessage('string body', 'fb')).toBe('fb')
  })
})

describe('createBizError', () => {
  it('挂 status / bizCode 机器读字段', () => {
    const e = createBizError('msg', { status: 403 }, 4301)
    expect(e.message).toBe('msg')
    expect(e.status).toBe(403)
    expect(e.bizCode).toBe(4301)
  })
  it('缺省 response ⇒ status 0；未传 bizCode 不挂字段', () => {
    const e = createBizError('m', null)
    expect(e.status).toBe(0)
    expect('bizCode' in e).toBe(false)
  })
  it('markReported 钩子生效（应用侧上报注入）', () => {
    const e = createBizError('m', null, 1, (x) => ({ ...x, reported: true }))
    expect(e.reported).toBe(true)
  })
})

describe('业务码常量', () => {
  it('4301/4305 与 user-web 旧值一致（迁移契约）', () => {
    expect(BIZ_CODE_MODULE_NOT_ENABLED).toBe(4301)
    expect(BIZ_CODE_SUBSCRIPTION_EXPIRED).toBe(4305)
  })
})

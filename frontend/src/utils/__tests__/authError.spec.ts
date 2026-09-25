import { describe, expect, it, vi } from 'vitest'
import { buildAuthErrorMessage } from '@/utils/authError'

describe('buildAuthErrorMessage', () => {
  it('prefers response detail message when available', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            detail: 'detailed message',
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('detailed message')
  })

  it('falls back to response message when detail is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('plain message')
  })

  it('falls back to error.message when response payload is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        message: 'error message'
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('error message')
  })

  it('uses fallback when no message can be extracted', () => {
    expect(buildAuthErrorMessage({}, { fallback: 'fallback' })).toBe('fallback')
  })

  it.each([
    [{ reason: 'EMAIL_EXISTS', message: 'email already exists' }, 'email already exists'],
    [{ reason: 'USER_NOT_ACTIVE', message: '账户被暂停，请联系管理员' }, '账户被暂停，请联系管理员'],
    [{ message: 'Request failed with status code 400', detail: '验证码已过期，请重新发送' }, '验证码已过期，请重新发送'],
    [{ message: 'Request failed with status code 429', error: { message: '请在 35 秒后重试' } }, '请在 35 秒后重试'],
    [{ message: 'Request failed with status code 409', response: { data: { error: 'email already exists' } } }, 'email already exists'],
    ['邮件服务暂不可用', '邮件服务暂不可用'],
    [{ message: '  原始原因\n请稍后重试  ' }, '  原始原因\n请稍后重试  ']
  ])('保留不同响应形式中的原始错误：%j', (error, expected) => {
    const fallback = vi.fn(() => '固定错误码提示')
    expect(buildAuthErrorMessage(error, { fallback })).toBe(expected)
    expect(fallback).not.toHaveBeenCalled()
  })

  it.each([undefined, null, {}, { message: '   ' }, { detail: {}, message: 123, error: {} }])(
    '原始信息缺失或不可显示时才调用兜底：%j', (error) => {
      const fallback = vi.fn(() => '登录失败，请重试')
      expect(buildAuthErrorMessage(error, { fallback })).toBe('登录失败，请重试')
      expect(fallback).toHaveBeenCalledOnce()
    }
  )
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiRequestError, getActor, setActor } from './client'

describe('actor storage', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('defaults to "api" when nothing is stored', () => {
    expect(getActor()).toBe('api')
  })

  it('persists and returns a custom actor', () => {
    setActor('nanor')
    expect(getActor()).toBe('nanor')
  })

  it('falls back to "api" when set with an empty string', () => {
    setActor('')
    expect(getActor()).toBe('api')
  })
})

describe('ApiRequestError.body', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('carries the full parsed JSON body of a non-2xx response, not just its message', async () => {
    const rejection = { message: 'reply-guard rejected the candidate reply: verdict=revise', verdict: 'revise', findings: [{ policy: 'tone', severity: 'medium', issue: 'curt', quote: 'no' }] }
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(rejection), { status: 409, headers: { 'Content-Type': 'application/json' } }),
      ),
    )

    await expect(api.addComment('tkt_1', { author: 'a', body: 'no', internal: false })).rejects.toSatisfy((e: unknown) => {
      expect(e).toBeInstanceOf(ApiRequestError)
      const err = e as ApiRequestError
      expect(err.status).toBe(409)
      expect(err.body).toEqual(rejection)
      return true
    })
  })
})

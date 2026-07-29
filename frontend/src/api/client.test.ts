import { beforeEach, describe, expect, it } from 'vitest'
import { getActor, setActor } from './client'

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

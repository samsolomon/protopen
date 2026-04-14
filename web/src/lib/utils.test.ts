import { describe, it, expect } from 'vitest'
import { commitURL } from './utils'

describe('commitURL', () => {
  it('generates GitHub commit URL', () => {
    expect(commitURL('https://github.com/user/repo', 'abc123'))
      .toBe('https://github.com/user/repo/commit/abc123')
  })

  it('generates GitLab commit URL', () => {
    expect(commitURL('https://gitlab.com/user/repo', 'abc123'))
      .toBe('https://gitlab.com/user/repo/-/commit/abc123')
  })

  it('generates Bitbucket commit URL', () => {
    expect(commitURL('https://bitbucket.org/user/repo', 'abc123'))
      .toBe('https://bitbucket.org/user/repo/commits/abc123')
  })

  it('handles self-hosted GitLab', () => {
    expect(commitURL('https://gitlab.internal.co/team/repo', 'abc123'))
      .toBe('https://gitlab.internal.co/team/repo/-/commit/abc123')
  })

  it('falls back to /commit/ for unknown hosts', () => {
    expect(commitURL('https://git.example.com/repo', 'abc123'))
      .toBe('https://git.example.com/repo/commit/abc123')
  })

  it('falls back to /commit/ for invalid URLs', () => {
    expect(commitURL('not-a-url', 'abc123'))
      .toBe('not-a-url/commit/abc123')
  })
})

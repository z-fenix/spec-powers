import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  listPropertyDefinitions,
  createPropertyDefinition,
  updatePropertyDefinition,
  deletePropertyDefinition,
  listIssueProperties,
  setIssueProperty,
  deleteIssueProperty,
  listProjectIssueProperties,
  encodeMultiSelect,
  decodeMultiSelect,
} from './properties'
import { apiFetch } from './client'

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockedFetch = vi.mocked(apiFetch)

const def = {
  id: 'prop1',
  project_id: 'p1',
  name: '模块',
  type: 'select',
  options: ['前端', '后端'],
  position: 0,
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedFetch.mockReset()
})

describe('properties api', () => {
  it('lists property definitions', async () => {
    mockedFetch.mockResolvedValueOnce({ properties: [def] })
    await expect(listPropertyDefinitions('p1')).resolves.toEqual([def])
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/properties')
  })

  it('creates a property definition', async () => {
    mockedFetch.mockResolvedValueOnce({ property: def })
    const input = { name: '模块', type: 'select', options: ['前端', '后端'] }
    await expect(createPropertyDefinition('p1', input)).resolves.toEqual(def)
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/properties', {
      method: 'POST',
      body: input,
    })
  })

  it('updates a property definition', async () => {
    mockedFetch.mockResolvedValueOnce({ property: def })
    const input = { name: '模块', type: 'select', options: ['a'] }
    await expect(updatePropertyDefinition('p1', 'prop1', input)).resolves.toEqual(def)
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/properties/prop1', {
      method: 'PATCH',
      body: input,
    })
  })

  it('deletes a property definition', async () => {
    mockedFetch.mockResolvedValueOnce(undefined)
    await deletePropertyDefinition('p1', 'prop1')
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/properties/prop1', {
      method: 'DELETE',
    })
  })

  it('lists and sets issue property values', async () => {
    const value = { issue_id: 'i1', property_id: 'prop1', value: '后端' }
    mockedFetch.mockResolvedValueOnce({ values: [value] })
    await expect(listIssueProperties('p1', 'i1')).resolves.toEqual([value])
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/properties')

    mockedFetch.mockResolvedValueOnce({ value })
    await expect(setIssueProperty('p1', 'i1', 'prop1', '后端')).resolves.toEqual(value)
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/properties/prop1', {
      method: 'PUT',
      body: { value: '后端' },
    })

    mockedFetch.mockResolvedValueOnce(undefined)
    await deleteIssueProperty('p1', 'i1', 'prop1')
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/issues/i1/properties/prop1', {
      method: 'DELETE',
    })
  })

  it('lists project-wide issue property values', async () => {
    mockedFetch.mockResolvedValueOnce({ values: [] })
    await expect(listProjectIssueProperties('p1')).resolves.toEqual([])
    expect(apiFetch).toHaveBeenCalledWith('/projects/p1/properties/values')
  })

  it('encodes and decodes multi_select JSON values', () => {
    expect(encodeMultiSelect(['a', 'b'])).toBe('["a","b"]')
    expect(decodeMultiSelect('["a","b"]')).toEqual(['a', 'b'])
    expect(decodeMultiSelect('not json')).toEqual([])
    expect(decodeMultiSelect('42')).toEqual([])
  })
})

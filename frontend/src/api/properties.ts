import { apiFetch } from './client'

export type PropertyType =
  | 'select'
  | 'multi_select'
  | 'checkbox'
  | 'text'
  | 'number'
  | 'date'

export const PROPERTY_TYPES: PropertyType[] = [
  'select',
  'multi_select',
  'checkbox',
  'text',
  'number',
  'date',
]

export interface PropertyDefinition {
  id: string
  project_id: string
  name: string
  type: PropertyType | string
  options: string[]
  position: number
}

export interface IssuePropertyValue {
  issue_id: string
  property_id: string
  value: string
}

export interface PropertyDefinitionInput {
  name: string
  type: string
  options?: string[]
}

export function listPropertyDefinitions(projectId: string): Promise<PropertyDefinition[]> {
  return apiFetch<{ properties: PropertyDefinition[] }>(`/projects/${projectId}/properties`).then(
    (res) => res.properties ?? [],
  )
}

export function createPropertyDefinition(
  projectId: string,
  input: PropertyDefinitionInput,
): Promise<PropertyDefinition> {
  return apiFetch<{ property: PropertyDefinition }>(`/projects/${projectId}/properties`, {
    method: 'POST',
    body: input,
  }).then((res) => res.property)
}

export function updatePropertyDefinition(
  projectId: string,
  propertyId: string,
  input: PropertyDefinitionInput,
): Promise<PropertyDefinition> {
  return apiFetch<{ property: PropertyDefinition }>(
    `/projects/${projectId}/properties/${propertyId}`,
    { method: 'PATCH', body: input },
  ).then((res) => res.property)
}

export function deletePropertyDefinition(projectId: string, propertyId: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/properties/${propertyId}`, {
    method: 'DELETE',
  })
}

export function listIssueProperties(
  projectId: string,
  issueId: string,
): Promise<IssuePropertyValue[]> {
  return apiFetch<{ values: IssuePropertyValue[] }>(
    `/projects/${projectId}/issues/${issueId}/properties`,
  ).then((res) => res.values ?? [])
}

export function setIssueProperty(
  projectId: string,
  issueId: string,
  propertyId: string,
  value: string,
): Promise<IssuePropertyValue> {
  return apiFetch<{ value: IssuePropertyValue }>(
    `/projects/${projectId}/issues/${issueId}/properties/${propertyId}`,
    { method: 'PUT', body: { value } },
  ).then((res) => res.value)
}

export function deleteIssueProperty(
  projectId: string,
  issueId: string,
  propertyId: string,
): Promise<void> {
  return apiFetch<void>(
    `/projects/${projectId}/issues/${issueId}/properties/${propertyId}`,
    { method: 'DELETE' },
  )
}

export function listProjectIssueProperties(projectId: string): Promise<IssuePropertyValue[]> {
  return apiFetch<{ values: IssuePropertyValue[] }>(
    `/projects/${projectId}/properties/values`,
  ).then((res) => res.values ?? [])
}

export function encodeMultiSelect(values: string[]): string {
  return JSON.stringify(values)
}

export function decodeMultiSelect(value: string): string[] {
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === 'string') : []
  } catch {
    return []
  }
}

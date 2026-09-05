import { apiFetch } from './client'

export interface Webhook {
  id: string
  name: string
  secret: string
  enabled: boolean
  created_at: string
}

export async function listWebhooks(): Promise<Webhook[]> {
  const res = await apiFetch<{ webhooks: Webhook[] }>('/webhooks')
  return res.webhooks ?? []
}

export async function createWebhook(name: string): Promise<Webhook> {
  const res = await apiFetch<{ webhook: Webhook }>('/webhooks', {
    method: 'POST',
    body: { name },
  })
  return res.webhook
}

export async function updateWebhook(
  id: string,
  patch: { name?: string; enabled?: boolean },
): Promise<Webhook> {
  const res = await apiFetch<{ webhook: Webhook }>(`/webhooks/${id}`, {
    method: 'PATCH',
    body: patch,
  })
  return res.webhook
}

export async function deleteWebhook(id: string): Promise<void> {
  await apiFetch(`/webhooks/${id}`, { method: 'DELETE' })
}

export type DeliveryStatus = 'pending' | 'deferred' | 'delivered' | 'bounced' | 'forwarded'
export type DeliveryEventStatus = DeliveryStatus | 'accepted' | 'queued' | 'removed'

export type Delivery = {
  id: number
  queue_id: string
  message_id: string
  sender: string
  recipient: string
  original_recipient: string
  status: DeliveryStatus
  first_seen_at: string
  last_seen_at: string
  relay: string
  dsn: string
  response: string
}

export type DeliveryEvent = {
  id: number
  at: string
  status: DeliveryEventStatus
  relay: string
  dsn: string
  response: string
  delay_seconds?: number | null
}

export type CollectorStatus = {
  state: 'starting' | 'ok' | 'error'
  warning: '' | 'history_gap' | 'source_unavailable'
  last_collected_at: string | null
}

export type DeliveryList = {
  items: Delivery[]
  total: number
  page: number
  page_size: number
  collector: CollectorStatus
}

export type DeliveryDetail = {
  delivery: Delivery
  events: DeliveryEvent[]
  collector: CollectorStatus
  events_truncated?: boolean
}

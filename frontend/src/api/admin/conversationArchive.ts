export interface ArchiveRecord {
  id: number
  request_id: string
  user_id: number | null
  api_key_id: number | null
  session_id: string
  endpoint: string
  model: string
  upstream_model?: string
  response_model?: string
  wire_model?: string
  model_conflict?: boolean
  reply_source?: string
  http_status: number
  status: 'complete' | 'interrupted' | 'error'
  truncated: boolean
  created_at: string
  user_name?: string
  user_email?: string
  prompt_preview?: string
  response_preview?: string
  blocked?: boolean
  auto_reply?: boolean
  prompt_omitted?: boolean
  request_body?: unknown
  response_body?: unknown
  conversation_key?: string
  group_source?: string
  call_count?: number
  models?: string[]
  first_at?: string
  blocked_count?: number
  auto_reply_count?: number
}

export interface ArchivePage {
  items: ArchiveRecord[]
  next_before_id: number
  capture_enabled: boolean
  capture_paused?: boolean
  retention_days?: number
}

export interface ApiPage<TItem> {
  items: TItem[];
  limit: number;
  offset: number;
}

export interface ProjectSummary {
  id: string;
  name: string;
  description: string;
  status: string;
  brain_id?: string;
  human_review_required: boolean;
}

export interface TaskSummary {
  id: string;
  project_id: string;
  title: string;
  status: string;
  priority: string;
  assigned_to?: string;
  updated_at?: string;
}

export interface TaskAuditEvent {
  id: string;
  event_type: string;
  actor_type: string;
  actor_id: string;
  from_status?: string;
  to_status?: string;
  reason?: string;
  created_at: string;
}

export interface TaskDetail extends TaskSummary {
  description: string;
  human_review_required: boolean;
  created_at: string;
  updated_at: string;
  audit_events: TaskAuditEvent[];
}

export interface TaskConversationMessage {
  id: string;
  sender_type: 'USER' | 'AGENT' | 'BRAIN' | 'SYSTEM';
  sender_id: string;
  content: string;
  content_type: string;
  created_at: string;
}

export interface TaskConversation {
  id: string;
  project_id: string;
  task_id: string;
  messages: TaskConversationMessage[];
  next_before?: string;
}

export interface AgentSummary {
  id: string;
  name: string;
  status: string;
  project_ids?: string[];
}

export type AgentRuntime = 'API' | 'MCP' | 'LOCAL' | 'REMOTE';

export interface Agent {
  id: string;
  name: string;
  avatar: string;
  description: string;
  role: string;
  runtime: AgentRuntime;
  provider?: string;
  provider_base_url?: string;
  model?: string;
  credential_id?: string;
  instructions: string;
  language_preferences: string[];
  capabilities: string[];
  status: string;
}

export interface AgentConfiguration {
  name: string;
  avatar: string;
  description: string;
  role: string;
  runtime: AgentRuntime;
  provider?: string;
  provider_base_url?: string;
  model?: string;
  credential_id?: string;
  instructions: string;
  language_preferences: string[];
  capabilities: string[];
}

export interface CredentialMetadata {
  id: string;
  name: string;
  provider: string;
}
export interface BrainAgent extends AgentSummary {
  role: string;
  runtime: 'API' | 'MCP' | 'LOCAL' | 'REMOTE';
  provider?: string;
  model?: string;
  capabilities: string[];
}
export interface BrainSummary {
  id: string;
  project_id: string;
  agent_id: string;
  autonomy: 'MANUAL' | 'SEMI_AUTO' | 'AUTONOMOUS';
  active: boolean;
}

export interface BrainConfiguration {
  agent_id: string;
  autonomy: 'MANUAL' | 'SEMI_AUTO' | 'AUTONOMOUS';
}

export interface RunSummary {
  id: string;
  project_id: string;
  task_id?: string;
  agent_id: string;
  runtime?: string;
  model: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  input?: string;
  output?: string;
  error?: string;
  audit_events?: RunAuditEvent[];
}

export interface NotificationNotice {
  id: string;
  project_id: string;
  event_type: string;
  resource_id?: string;
  severity: 'INFO' | 'WARNING' | 'CRITICAL';
  summary: string;
  created_at: string;
  delivery_state: 'PENDING' | 'DELIVERED' | 'FAILED';
  delivery_attempts: number;
  delivery_error?: string;
  read_at?: string;
  acknowledged_at?: string;
}

export interface RunAuditEvent {
  id: string;
  event_type: string;
  actor_type: string;
  actor_id: string;
  from_status?: string;
  to_status?: string;
  created_at: string;
}

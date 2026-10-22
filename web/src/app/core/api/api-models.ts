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
  assigned_to?: string;
}

export interface AgentSummary {
  id: string;
  name: string;
  status: string;
}

export interface BrainSummary {
  id: string;
  project_id: string;
  agent_id: string;
  autonomy: string;
  active: boolean;
}

export interface RunSummary {
  id: string;
  project_id: string;
  task_id?: string;
  agent_id: string;
  model: string;
  status: string;
}

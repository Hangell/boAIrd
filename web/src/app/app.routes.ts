import type {} from '@angular/localize';
import { Routes } from '@angular/router';
import { localSessionGuard } from './core/auth/local-session.guard';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./features/auth/local-login.component').then((module) => module.LocalLoginComponent),
    title: 'Local administrator session',
  },
  { path: '', pathMatch: 'full', redirectTo: 'overview' },
  {
    path: 'overview',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/projects/project-list.component').then((module) => module.ProjectListComponent),
    title: $localize`:@@page.overview.title:Overview`,
    data: {
      title: $localize`:@@page.overview.heading:Project overview`,
      description: $localize`:@@page.overview.description:Monitor project activity, task progress, and agent availability from one workspace.`,
    },
  },
  {
    path: 'projects',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/projects/project-list.component').then((module) => module.ProjectListComponent),
    title: $localize`:@@page.projects.title:Projects`,
    data: {
      title: $localize`:@@page.projects.heading:Projects`,
      description: $localize`:@@page.projects.description:Browse and manage your boAIrd projects.`,
    },
  },
  {
    path: 'projects/:projectId',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/projects/project-dashboard.component').then((module) => module.ProjectDashboardComponent),
    title: $localize`:@@page.project.title:Project details`,
    data: {
      title: $localize`:@@page.project.heading:Project workspace`,
      description: $localize`:@@page.project.description:Review project configuration and current work.`,
    },
  },
  {
    path: 'tasks',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/tasks/task-board.component').then((module) => module.TaskBoardComponent),
    title: $localize`:@@page.tasks.title:Kanban`,
    data: {
      title: $localize`:@@page.tasks.heading:Task board`,
      description: $localize`:@@page.tasks.description:Track backend-owned task status across the project workflow.`,
    },
  },
  {
    path: 'conversations',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./shared/feature-page.component').then((module) => module.FeaturePageComponent),
    title: $localize`:@@page.conversations.title:Conversations`,
    data: {
      title: $localize`:@@page.conversations.heading:Conversations`,
      description: $localize`:@@page.conversations.description:Review the full message history attached to project tasks.`,
    },
  },
  {
    path: 'agents/new',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/agents/agent-editor.component').then((module) => module.AgentEditorComponent),
    title: $localize`:@@page.agentCreate.title:Create agent`,
  },
  {
    path: 'agents/:agentId',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/agents/agent-editor.component').then((module) => module.AgentEditorComponent),
    title: $localize`:@@page.agent.title:Agent details`,
  },
  {
    path: 'agents',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/agents/agent-list.component').then((module) => module.AgentListComponent),
    title: $localize`:@@page.agents.title:Agents`,
    data: {
      title: $localize`:@@page.agents.heading:Agents`,
      description: $localize`:@@page.agents.description:Inspect configured agents and their explicit capabilities.`,
    },
  },
  {
    path: 'providers',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./shared/feature-page.component').then((module) => module.FeaturePageComponent),
    title: $localize`:@@page.providers.title:Providers`,
    data: {
      title: $localize`:@@page.providers.heading:Providers`,
      description: $localize`:@@page.providers.description:Inspect configured model providers and their available models.`,
    },
  },
  {
    path: 'runs',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/observability/observability.component').then((module) => module.ObservabilityComponent),
    title: $localize`:@@page.runs.title:Runs`,
    data: {
      view: 'runs',
      title: $localize`:@@page.runs.heading:Run history`,
      description: $localize`:@@page.runs.description:Review model execution status and outputs.`,
    },
  },
  {
    path: 'logs',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/observability/observability.component').then((module) => module.ObservabilityComponent),
    title: $localize`:@@page.logs.title:Logs`,
    data: {
      view: 'logs',
      title: $localize`:@@page.logs.heading:Activity logs`,
      description: $localize`:@@page.logs.description:Inspect project and task activity.`,
    },
  },
  {
    path: 'brain',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./features/brain/brain-dashboard.component').then((module) => module.BrainDashboardComponent),
    title: $localize`:@@page.brain.title:Brain`,
    data: {
      title: $localize`:@@page.brain.heading:Project Brain`,
      description: $localize`:@@page.brain.description:Review the Brain assigned to coordinate project planning.`,
    },
  },
  {
    path: 'settings',
    canActivate: [localSessionGuard],
    loadComponent: () => import('./shared/feature-page.component').then((module) => module.FeaturePageComponent),
    title: $localize`:@@page.settings.title:Settings`,
    data: {
      title: $localize`:@@page.settings.heading:Settings`,
      description: $localize`:@@page.settings.description:Configure the local dashboard connection and workspace preferences.`,
    },
  },
  { path: '**', redirectTo: 'overview' },
];

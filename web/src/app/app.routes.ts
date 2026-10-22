import type {} from '@angular/localize';
import { Routes } from '@angular/router';
import { FeaturePageComponent } from './shared/feature-page.component';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'overview' },
  {
    path: 'overview',
    component: FeaturePageComponent,
    title: $localize`:@@page.overview.title:Overview`,
    data: {
      title: $localize`:@@page.overview.heading:Project overview`,
      description: $localize`:@@page.overview.description:Monitor project activity, task progress, and agent availability from one workspace.`,
    },
  },
  {
    path: 'projects',
    component: FeaturePageComponent,
    title: $localize`:@@page.projects.title:Projects`,
    data: {
      title: $localize`:@@page.projects.heading:Projects`,
      description: $localize`:@@page.projects.description:Browse and manage your boAIrd projects.`,
    },
  },
  {
    path: 'projects/:projectId',
    component: FeaturePageComponent,
    title: $localize`:@@page.project.title:Project details`,
    data: {
      title: $localize`:@@page.project.heading:Project workspace`,
      description: $localize`:@@page.project.description:Review project configuration and current work.`,
    },
  },
  {
    path: 'tasks',
    component: FeaturePageComponent,
    title: $localize`:@@page.tasks.title:Kanban`,
    data: {
      title: $localize`:@@page.tasks.heading:Task board`,
      description: $localize`:@@page.tasks.description:Track backend-owned task status across the project workflow.`,
    },
  },
  {
    path: 'conversations',
    component: FeaturePageComponent,
    title: $localize`:@@page.conversations.title:Conversations`,
    data: {
      title: $localize`:@@page.conversations.heading:Conversations`,
      description: $localize`:@@page.conversations.description:Review the full message history attached to project tasks.`,
    },
  },
  {
    path: 'agents',
    component: FeaturePageComponent,
    title: $localize`:@@page.agents.title:Agents`,
    data: {
      title: $localize`:@@page.agents.heading:Agents`,
      description: $localize`:@@page.agents.description:Inspect configured agents and their explicit capabilities.`,
    },
  },
  {
    path: 'providers',
    component: FeaturePageComponent,
    title: $localize`:@@page.providers.title:Providers`,
    data: {
      title: $localize`:@@page.providers.heading:Providers`,
      description: $localize`:@@page.providers.description:Inspect configured model providers and their available models.`,
    },
  },
  {
    path: 'runs',
    component: FeaturePageComponent,
    title: $localize`:@@page.runs.title:Runs`,
    data: {
      title: $localize`:@@page.runs.heading:Run history`,
      description: $localize`:@@page.runs.description:Review model execution status and outputs.`,
    },
  },
  {
    path: 'logs',
    component: FeaturePageComponent,
    title: $localize`:@@page.logs.title:Logs`,
    data: {
      title: $localize`:@@page.logs.heading:Activity logs`,
      description: $localize`:@@page.logs.description:Inspect project and task activity.`,
    },
  },
  {
    path: 'brain',
    component: FeaturePageComponent,
    title: $localize`:@@page.brain.title:Brain`,
    data: {
      title: $localize`:@@page.brain.heading:Project Brain`,
      description: $localize`:@@page.brain.description:Review the Brain assigned to coordinate project planning.`,
    },
  },
  {
    path: 'settings',
    component: FeaturePageComponent,
    title: $localize`:@@page.settings.title:Settings`,
    data: {
      title: $localize`:@@page.settings.heading:Settings`,
      description: $localize`:@@page.settings.description:Configure the local dashboard connection and workspace preferences.`,
    },
  },
  { path: '**', redirectTo: 'overview' },
];

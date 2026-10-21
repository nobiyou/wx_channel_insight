import { monitoringTasksForScope } from './operationsNavigation.js';

export function buildOperationsSummary(tasks = [], savedAccounts = null, videos = null) {
  const available = Array.isArray(tasks);
  const plans = available ? tasks.filter(task => task.status !== 'archived') : [];
  const accounts = monitoringTasksForScope(plans, 'accounts');
  const keywords = monitoringTasksForScope(plans, 'keywords');
  const keywordKeys = status => new Set(keywords.filter(task => !status || task.status === status).flatMap(task => task.targets.map(target => target.targetKey)));
  const enabledKeywords = keywordKeys('enabled');
  const pausedKeywords = new Set([...keywordKeys('paused')].filter(key => !enabledKeywords.has(key)));
  const count = (items, status) => available ? items.filter(task => task.status === status).length : null;
  return {
    savedAccounts,
    videos,
    accountPlans: available ? accounts.length : null,
    accountEnabled: count(accounts, 'enabled'),
    accountPaused: count(accounts, 'paused'),
    keywords: available ? keywordKeys().size : null,
    keywordEnabled: available ? enabledKeywords.size : null,
    keywordPaused: available ? pausedKeywords.size : null,
  };
}

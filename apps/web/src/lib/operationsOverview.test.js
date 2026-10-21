import test from 'node:test';
import assert from 'node:assert/strict';
import { buildOperationsSummary } from './operationsOverview.js';
import { findMonitoringObjectTask } from './monitoringTasks.js';

const tasks = [
  {id:'account',taskType:'legacy',status:'paused',scopeMode:'selected',targets:[{source:'wx_feed_list',targetKey:'cctv@finder'}]},
  {id:'keyword',taskType:'legacy',status:'paused',scopeMode:'selected',targets:[{source:'wx_video_search',targetKey:'秋天'}]},
  {id:'default',system:true,scopeMode:'all_saved_bindings',status:'enabled',targets:[]},
];

test('overview separates one account monitor from 63 collected accounts', () => {
  const result = buildOperationsSummary(tasks, 63, 66);
  assert.equal(result.accountPlans, 1);
  assert.equal(result.savedAccounts, 63);
  assert.equal(result.accountEnabled, 0);
  assert.equal(result.accountPaused, 1);
  assert.equal(result.keywords, 1);
  assert.equal(result.keywordPaused, 1);
  assert.equal(result.videos, 66);
});

test('overview counts repeated keyword targets once and excludes archived plans', () => {
  const result = buildOperationsSummary([...tasks,
    {...tasks[1],id:'another-keyword',status:'enabled'},
    {...tasks[0],id:'archived',status:'archived'},
  ]);
  assert.equal(result.keywords, 1);
  assert.equal(result.keywordEnabled, 1);
  assert.equal(result.keywordPaused, 0);
  assert.equal(result.accountPlans, 1);
  assert.equal(result.savedAccounts, null);
});

test('adding an existing paused object reuses its plan and preserves pause', () => {
  const match = findMonitoringObjectTask(tasks, 'wx_feed_list', 'cctv@finder');
  assert.equal(match.id, 'account');
  assert.equal(match.status, 'paused');
  assert.equal(findMonitoringObjectTask(tasks, 'wx_video_search', '秋天').id, 'keyword');
  assert.equal(findMonitoringObjectTask(tasks, 'wx_video_search', '秋'), null);
});

test('existing-object lookup never replaces an archived or multi-object plan', () => {
  const candidates = [
    {...tasks[0],status:'archived'},
    {...tasks[0],id:'multi',targets:[...tasks[0].targets,{source:'wx_feed_list',targetKey:'other@finder'}]},
    {...tasks[0],id:'profile',taskType:'account_profile'},
  ];
  assert.equal(findMonitoringObjectTask(candidates, 'wx_feed_list', 'cctv@finder'), null);
});

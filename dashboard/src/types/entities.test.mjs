import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { hasRunThread, isActiveRun, runsPollInterval, STATELESS_THREAD_ID } from './entities.ts'

test('v2 status transitions determine live refresh cadence', () => {
  for (const status of ['pending', 'running']) {
    assert.equal(isActiveRun({ status }), true)
    assert.equal(runsPollInterval([{ status }]), 1500)
  }
  for (const status of ['success', 'error', 'interrupted', 'timeout']) {
    assert.equal(isActiveRun({ status }), false)
    assert.equal(runsPollInterval([{ status }]), 15000)
  }
  assert.equal(runsPollInterval([]), 15000)
  assert.equal(runsPollInterval(undefined), 15000)
})

test('stateless run never receives a fake thread link', () => {
  assert.equal(hasRunThread({ thread_id: STATELESS_THREAD_ID }), false)
  assert.equal(hasRunThread({ thread_id: '' }), false)
  assert.equal(hasRunThread({ thread_id: '1f1bc173-af09-482f-a7be-f6a614544731' }), true)
})

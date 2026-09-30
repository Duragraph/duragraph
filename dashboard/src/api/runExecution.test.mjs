import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { applyExecutionEvent, emptyExecution, graphTopology } from './runExecution.ts'

test('only an actual topology in GraphSchema produces graph nodes', () => {
  assert.equal(graphTopology({ state_schema: { type: 'object' } }), null)
  assert.equal(graphTopology({ state_schema: { nodes: [{ id: 'A' }], edges: [] } }), null)
  assert.deepEqual(graphTopology({ state_schema: { nodes: [{ id: 'A', type: 'tool' }], edges: [] } })?.nodes.map(n => n.id), ['A'])
})

test('run events replay node progress and real input/output without Run DTO fields', () => {
  let state = emptyExecution()
  state = applyExecutionEvent(state, 'run.created', { input: { message: 'hi' } })
  state = applyExecutionEvent(state, 'execution.node_started', { node_id: 'A' })
  assert.equal(state.nodes.A.status, 'running')
  state = applyExecutionEvent(state, 'execution.node_completed', { node_id: 'A', output: { ok: true }, duration_ms: 37 })
  state = applyExecutionEvent(state, 'run.completed', { output: { result: 'ok' } })
  assert.deepEqual(state.input, { message: 'hi' })
  assert.deepEqual(state.output, { result: 'ok' })
  assert.equal(state.nodes.A.durationMs, 37)
  assert.equal(state.nodes.A.status, 'completed')
  assert.equal(applyExecutionEvent(state, 'heartbeat', {}), state)
})

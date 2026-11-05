import test from 'node:test';
import assert from 'node:assert/strict';
import { displaySpec } from '../ui/lib/spec.js';
import { speakerControls } from '../ui/lib/speaker.js';

const payload = {
  language: 'zh_cn', translations: null,
  spec: { type: 'urn:miot-spec-v2:device:speaker:0:example:1', description: 'Speaker', services: [
    { iid: 8, type: 'urn:miot-spec-v2:service:speaker:0', description: 'Speaker', properties: [
      { iid: 9, type: 'urn:miot-spec-v2:property:volume:0', description: 'Volume', format: 'uint8', access: ['read', 'write'], 'value-range': [5, 100, 1] },
    ] },
    { iid: 12, type: 'urn:miot-spec-v2:service:play-control:0', description: 'Playback', properties: [
      { iid: 3, type: 'urn:miot-spec-v2:property:playing-state:0', description: 'State', format: 'uint8', access: ['notify'] },
    ], actions: [
      { iid: 7, type: 'urn:miot-spec-v2:action:play:0', description: 'Play' },
      { iid: 9, type: 'urn:miot-spec-v2:action:pause:0', description: 'Pause' },
    ] },
    { iid: 15, type: 'urn:miot-spec-v2:service:intelligent-speaker:0', description: 'Speech', properties: [
      { iid: 22, type: 'urn:miot-spec-v2:property:text-content:0', description: 'Text', format: 'string', access: [] },
    ], actions: [{ iid: 8, type: 'urn:miot-spec-v2:action:play-text:0', description: 'Speak', in: [22] }] },
  ] },
};

test('speaker controls retain the declared IDs, volume range and supported actions', () => {
  const before = JSON.stringify(payload);
  const { playback, speech } = speakerControls(displaySpec(payload));
  assert.deepEqual(playback.services.map(s => s.iid), [8, 12]);
  const volume = playback.services[0].properties[0];
  assert.equal(volume.iid, 9);
  assert.deepEqual(volume.range, { min: 5, max: 100, step: 1 });
  assert.deepEqual(playback.services[1].actions.map(a => a.type), ['play', 'pause']);
  assert.deepEqual(playback.services[1].properties[0].access, ['notify']);
  assert.deepEqual(speech, { siid: 15, aiid: 8 });
  assert.equal(JSON.stringify(payload), before);
});

test('missing device specification uses the legacy controls', () => {
  assert.deepEqual(speakerControls(null), { playback: null, speech: null });
  assert.deepEqual(speakerControls({ services: [] }), { playback: null, speech: null });
});

test('speech actions with other required arguments cannot be used as a text-only call', () => {
  const spec = displaySpec(payload);
  spec.services[2].actions[0].in.push(23);
  assert.equal(speakerControls(spec).speech, null);
});

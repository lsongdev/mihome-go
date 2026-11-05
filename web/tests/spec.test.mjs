import test from 'node:test';
import assert from 'node:assert/strict';
import { displaySpec } from '../ui/lib/spec.js';

const document = {
  spec: {
    type: 'urn:miot-spec-v2:device:fan:0000:vendor:1', description: 'Fan', vendor: { flag: true },
    services: [{
      iid: 2, type: 'urn:miot-spec-v2:service:fan:0000', description: 'Fan control',
      properties: [
        { iid: 1, type: 'urn:miot-spec-v2:property:on:0000', description: 'Power', format: 'bool', access: ['read', 'write'], unit: 'none' },
        { iid: 2, type: 'urn:miot-spec-v2:property:fan-level:0000', description: 'Speed', format: 'uint8', 'value-list': [{ value: 5, description: 'Low' }, { value: 9, description: 'High' }] },
        { iid: 3, type: 'urn:miot-spec-v2:property:brightness:0000', description: 'Brightness', format: 'uint8', unit: 'percentage', 'value-range': [1, 100, 5] },
      ],
      actions: [{ iid: 1, type: 'urn:miot-spec-v2:action:toggle:0000', description: 'Toggle', in: [1], out: [2] }],
      events: [{ iid: 1007, type: 'urn:miot-spec-v2:event:fault:0000', description: 'Fault', arguments: [1, 3] }],
    }, { iid: 9, type: 'urn:miot-spec-v2:service:empty:0000', description: 'Empty' }],
  },
  translations: { data: {
    zh_cn: { 'service:002': '风扇', 'service:002:property:001': '开关', 'service:002:property:002:valuelist:000': '低档', 'service:002:action:001': '切换', 'service:002:event:1007': '故障' },
    ja: { 'service:002': 'ファン', 'service:002:property:001': '電源' },
  } },
  language: 'zh_cn',
};

test('UI translates every element without changing either original document', () => {
  const before = JSON.stringify(document);
  const result = displaySpec(document);
  const [service, empty] = result.services;
  assert.equal(result.urn, document.spec.type);
  assert.equal(result.type, 'fan');
  assert.equal(service.name, '风扇');
  assert.equal(service.properties[0].name, '开关');
  assert.equal(service.properties[0].unit, undefined);
  assert.deepEqual(service.properties[1].options, [{ value: 5, name: '低档' }, { value: 9, name: 'High' }]);
  assert.equal(service.actions[0].name, '切换');
  assert.deepEqual(service.actions[0].in, [1]);
  assert.deepEqual(service.actions[0].out, [2]);
  assert.equal(service.events[0].name, '故障');
  assert.deepEqual(service.events[0].arguments, [1, 3]);
  assert.deepEqual(service.properties[2].range, { min: 1, max: 100, step: 5 });
  assert.deepEqual(empty.properties, []);
  assert.deepEqual(empty.actions, []);
  assert.deepEqual(empty.events, []);
  assert.equal(JSON.stringify(document), before);
});

test('language changes reuse the same documents and untranslated names use original descriptions', () => {
  const result = displaySpec({ ...document, language: 'ja' });
  assert.equal(result.services[0].name, 'ファン');
  assert.equal(result.services[0].properties[0].name, '電源');
  assert.equal(result.services[0].properties[1].name, 'Speed');
  assert.equal(result.services[0].actions[0].name, 'Toggle');
});

test('missing translation download or language keeps the original controls', () => {
  for (const payload of [{ ...document, translations: null }, { ...document, language: 'missing' }]) {
    const result = displaySpec(payload);
    assert.equal(result.services[0].name, 'Fan control');
    assert.equal(result.services[0].properties[0].name, 'Power');
    assert.equal(result.services[0].actions[0].name, 'Toggle');
    assert.equal(result.services[0].events[0].name, 'Fault');
  }
});

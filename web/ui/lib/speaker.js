// Choose controls by declared capabilities, without model-specific ID tables.
export function speakerControls(spec) {
  if (!spec) return { playback: null, speech: null };
  const services = spec.services.filter(service =>
    ['speaker', 'play-control'].includes(service.type));
  const usable = services.some(service => service.actions.length ||
    service.properties.some(property => property.access.includes('write')));
  const intelligent = spec.services.find(service => service.type === 'intelligent-speaker');
  const speech = intelligent?.actions.find(action => action.type === 'play-text' && action.in.length === 1 &&
    intelligent.properties.some(property => property.iid === action.in[0] &&
      property.type === 'text-content' && property.format === 'string'));
  return {
    playback: usable ? { ...spec, services } : null,
    speech: speech ? { siid: intelligent.iid, aiid: speech.iid } : null,
  };
}

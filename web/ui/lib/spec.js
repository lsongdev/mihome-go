// Build a display model without changing the original upstream documents.
// Translation keys use instance IDs; value-list keys use the option's position.
export function displaySpec({ spec, translations, language }) {
  const names = translations?.data?.[language] ?? {};
  const id = value => String(value).padStart(3, '0');
  const type = urn => urn?.split(':')[3] ?? '';
  const name = (entry, key) => names[key] || entry.description;
  return {
    urn: spec.type,
    type: type(spec.type),
    name: spec.description,
    services: (spec.services ?? []).map(service => {
      const key = `service:${id(service.iid)}`;
      return {
        iid: service.iid, type: type(service.type), name: name(service, key),
        properties: (service.properties ?? []).map(property => {
          const pkey = `${key}:property:${id(property.iid)}`;
          const range = property['value-range'];
          return {
            iid: property.iid, type: type(property.type), name: name(property, pkey),
            format: property.format, access: property.access ?? [],
            unit: property.unit === 'none' ? undefined : property.unit,
            range: range?.length >= 2 ? {
              min: range[0], max: range[1], step: range[2] > 0 ? range[2] : 1,
            } : undefined,
            options: (property['value-list'] ?? []).map((option, index) => ({
              value: option.value, name: name(option, `${pkey}:valuelist:${id(index)}`),
            })),
          };
        }),
        actions: (service.actions ?? []).map(action => ({
          iid: action.iid, type: type(action.type), name: name(action, `${key}:action:${id(action.iid)}`),
          in: action.in ?? [], out: action.out ?? [],
        })),
        events: (service.events ?? []).map(event => ({
          iid: event.iid, type: type(event.type), name: name(event, `${key}:event:${id(event.iid)}`),
          arguments: event.arguments ?? [],
        })),
      };
    }),
  };
}

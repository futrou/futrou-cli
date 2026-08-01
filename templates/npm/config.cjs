'use strict';

const SCHEMA_URL = 'https://futrou.com/futrou.schema.json';

class Resource {
  constructor(properties = {}) {
    Object.assign(this, properties);
  }

  toJSON() {
    return omitEmpty(Object.fromEntries(Object.entries(this).map(([key, value]) => [key, serialize(value)])));
  }
}

class Serverlet extends Resource {}
class DNS extends Resource {}
class Proxy extends Resource {}
class Volume extends Resource {}
class Cron extends Resource {}

class Config {
  constructor(properties = {}) {
    this.$schema = properties.$schema || SCHEMA_URL;
    this.workspace = properties.workspace;
    this.project = properties.project;
    this.serverlets = toResources(properties.serverlets, Serverlet);
    this.dns = toResources(properties.dns, DNS);
    this.proxies = toResources(properties.proxies, Proxy);
    this.volumes = toResources(properties.volumes, Volume);
    this.crons = toResources(properties.crons, Cron);
  }

  serverlet(properties) { this.serverlets.push(asResource(properties, Serverlet)); return this; }
  dnsZone(properties) { this.dns.push(asResource(properties, DNS)); return this; }
  proxy(properties) { this.proxies.push(asResource(properties, Proxy)); return this; }
  volume(properties) { this.volumes.push(asResource(properties, Volume)); return this; }
  cron(properties) { this.crons.push(asResource(properties, Cron)); return this; }

  toJSON() {
    return omitEmpty({
      $schema: this.$schema,
      workspace: this.workspace,
      project: this.project,
      serverlets: this.serverlets,
      dns: this.dns,
      proxies: this.proxies,
      volumes: this.volumes,
      crons: this.crons,
    });
  }

  toJson(space = 2) { return JSON.stringify(this.toJSON(), null, space); }
  toJs() { return `export default ${this.toJson(2)};\n`; }
  toJsonSchema() { return require('./futrou.schema.json'); }
}

function asResource(value, Type) { return value instanceof Type ? value : new Type(value); }
function toResources(values, Type) { return (values || []).map(value => asResource(value, Type)); }
function serialize(value) {
  if (value && typeof value.toJSON === 'function') return value.toJSON();
  if (Array.isArray(value)) return value.map(serialize);
  if (value && typeof value === 'object') return omitEmpty(Object.fromEntries(Object.entries(value).map(([key, item]) => [key, serialize(item)])));
  return value;
}
function omitEmpty(object) {
  return Object.fromEntries(Object.entries(object).filter(([, value]) => value !== undefined && (!Array.isArray(value) || value.length > 0)));
}

module.exports = { Config, Serverlet, DNS, Proxy, Volume, Cron, SCHEMA_URL };

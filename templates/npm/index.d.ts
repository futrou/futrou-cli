export const SCHEMA_URL: 'https://futrou.com/futrou.schema.json';

export interface ResourceProperties { [key: string]: unknown }

export class Serverlet { constructor(properties?: ResourceProperties); toJSON(): ResourceProperties }
export class DNS { constructor(properties?: ResourceProperties); toJSON(): ResourceProperties }
export class Proxy { constructor(properties?: ResourceProperties); toJSON(): ResourceProperties }
export class Volume { constructor(properties?: ResourceProperties); toJSON(): ResourceProperties }
export class Cron { constructor(properties?: ResourceProperties); toJSON(): ResourceProperties }

export interface ConfigProperties {
  $schema?: string;
  workspace?: string;
  project?: string;
  serverlets?: Array<Serverlet | ResourceProperties>;
  dns?: Array<DNS | ResourceProperties>;
  proxies?: Array<Proxy | ResourceProperties>;
  volumes?: Array<Volume | ResourceProperties>;
  crons?: Array<Cron | ResourceProperties>;
}

export class Config {
  constructor(properties?: ConfigProperties);
  serverlet(properties: Serverlet | ResourceProperties): this;
  dnsZone(properties: DNS | ResourceProperties): this;
  proxy(properties: Proxy | ResourceProperties): this;
  volume(properties: Volume | ResourceProperties): this;
  cron(properties: Cron | ResourceProperties): this;
  toJSON(): ConfigProperties;
  toJson(space?: number): string;
  toJs(): string;
  toJsonSchema(): Record<string, unknown>;
}

// Names for the generated OpenAPI types (schema.d.ts, `npm run gen:api`).
import type { components } from './schema';

type Schemas = components['schemas'];

export type Cluster = Schemas['Cluster'];
export type Node = Schemas['Node'];
export type NodeList = Schemas['NodeList'];
export type NodeStatus = Schemas['NodeStatusValue'];
export type Stats = Schemas['Stats'];
export type StatsResponse = Schemas['StatsResponse'];
export type Point = Schemas['Point'];
export type ErrorBody = Schemas['Error'];
export type Envelope = Schemas['Envelope'];
export type Subscription = Schemas['Subscription'];
export type NodeStatusEvent = Schemas['NodeStatusEvent'];
export type NodeStatsEvent = Schemas['NodeStatsEvent'];

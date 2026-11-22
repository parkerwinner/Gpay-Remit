export * from "./api";
import type { paths, components } from "./api";

export type Paths = paths;
export type Components = components;
export type Schemas = components["schemas"];

// Core entity types
export type User = Schemas extends { User: infer U } ? U : Record<string, unknown>;
export type Remittance = Schemas extends { Remittance: infer R } ? R : Record<string, unknown>;
export type Invoice = Schemas extends { Invoice: infer I } ? I : Record<string, unknown>;
export type Webhook = Schemas extends { Webhook: infer W } ? W : Record<string, unknown>;

import {
  getProviderOptionMetadata,
  type ProviderOptionDefinition,
  type ProviderOptionProvider,
  type ProviderOptionValue,
} from "./providerOptionMetadata";

export type ProviderOptionValues = Record<string, ProviderOptionValue>;

function coerceNumber(value: ProviderOptionValue): number {
  if (typeof value === "number") {
    return value;
  }

  if (typeof value === "boolean") {
    return value ? 1 : 0;
  }

  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function createDefaultProviderOptionValues(
  provider: ProviderOptionProvider,
): ProviderOptionValues {
  return Object.fromEntries(
    getProviderOptionMetadata(provider).map((option) => [
      option.id,
      option.defaultValue,
    ]),
  );
}

export function coerceProviderOptionValue(
  option: ProviderOptionDefinition,
  value: ProviderOptionValue,
): ProviderOptionValue {
  if (option.type === "number") {
    return coerceNumber(value);
  }

  if (option.type === "boolean") {
    return typeof value === "boolean" ? value : value === "true";
  }

  return String(value);
}

export function setProviderOptionValue(
  values: ProviderOptionValues,
  option: ProviderOptionDefinition,
  value: ProviderOptionValue,
): ProviderOptionValues {
  return {
    ...values,
    [option.id]: coerceProviderOptionValue(option, value),
  };
}

export function resetProviderOptionValue(
  values: ProviderOptionValues,
  option: ProviderOptionDefinition,
): ProviderOptionValues {
  return {
    ...values,
    [option.id]: option.defaultValue,
  };
}

export function isProviderOptionDefault(
  option: ProviderOptionDefinition,
  values: ProviderOptionValues,
): boolean {
  return values[option.id] === option.defaultValue;
}

export function getChangedProviderOptionIds(
  provider: ProviderOptionProvider,
  values: ProviderOptionValues,
): string[] {
  return getProviderOptionMetadata(provider)
    .filter((option) => !isProviderOptionDefault(option, values))
    .map((option) => option.id);
}

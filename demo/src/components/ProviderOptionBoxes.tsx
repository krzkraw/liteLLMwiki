import { useMemo, useState } from "react";
import {
  getProviderOptionMetadata,
  type ProviderOptionDefinition,
  type ProviderOptionProvider,
  type ProviderOptionValue,
} from "../lib/providers/providerOptionMetadata";
import {
  createDefaultProviderOptionValues,
  isProviderOptionDefault,
  resetProviderOptionValue,
  setProviderOptionValue,
  type ProviderOptionValues,
} from "../lib/providers/providerOptionState";

export interface ProviderOptionBoxesProps {
  provider: ProviderOptionProvider;
  values?: Partial<ProviderOptionValues>;
  onValueChange?: (
    id: string,
    value: ProviderOptionValue,
    values: ProviderOptionValues,
  ) => void;
}

export function ProviderOptionBoxes({
  provider,
  values,
  onValueChange,
}: ProviderOptionBoxesProps) {
  const defaultValues = useMemo(
    () => createDefaultProviderOptionValues(provider),
    [provider],
  );
  const [localValues, setLocalValues] =
    useState<ProviderOptionValues>(defaultValues);
  const definitions = getProviderOptionMetadata(provider);
  const providedValues = Object.fromEntries(
    Object.entries(values ?? {}).filter((entry): entry is [string, ProviderOptionValue] => {
      return entry[1] !== undefined;
    }),
  );
  const mergedValues: ProviderOptionValues = {
    ...defaultValues,
    ...localValues,
    ...providedValues,
  };
  const groups = groupProviderOptions(definitions);

  function applyValue(option: ProviderOptionDefinition, value: ProviderOptionValue) {
    const nextValues = setProviderOptionValue(mergedValues, option, value);
    setLocalValues(nextValues);
    onValueChange?.(option.id, nextValues[option.id], nextValues);
  }

  function resetValue(option: ProviderOptionDefinition) {
    const nextValues = resetProviderOptionValue(mergedValues, option);
    setLocalValues(nextValues);
    onValueChange?.(option.id, nextValues[option.id], nextValues);
  }

  return (
    <div className="provider-option-groups">
      {groups.map(([group, options]) => (
        <section key={group} className="provider-option-group">
          <h3>{group}</h3>
          <div className="provider-option-grid">
            {options.map((option) => (
              <ProviderOptionPill
                key={option.id}
                option={option}
                value={mergedValues[option.id] ?? option.defaultValue}
                isDefault={isProviderOptionDefault(option, mergedValues)}
                onValueChange={applyValue}
                onReset={resetValue}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function groupProviderOptions(
  definitions: ProviderOptionDefinition[],
): Array<[string, ProviderOptionDefinition[]]> {
  const groups = new Map<string, ProviderOptionDefinition[]>();

  for (const definition of definitions) {
    const group = groups.get(definition.group) ?? [];
    group.push(definition);
    groups.set(definition.group, group);
  }

  return Array.from(groups.entries());
}

interface ProviderOptionPillProps {
  option: ProviderOptionDefinition;
  value: ProviderOptionValue;
  isDefault: boolean;
  onValueChange: (
    option: ProviderOptionDefinition,
    value: ProviderOptionValue,
  ) => void;
  onReset: (option: ProviderOptionDefinition) => void;
}

function ProviderOptionPill({
  option,
  value,
  isDefault,
  onValueChange,
  onReset,
}: ProviderOptionPillProps) {
  return (
    <label
      className="provider-option-pill"
      data-testid={`provider-option-pill-${option.id}`}
      title={`${option.tooltip} Source: ${option.source}`}
    >
      <span className="provider-option-label">
        {option.label}
        {option.requiresReload ? <small>reload</small> : null}
      </span>
      <ProviderOptionInput
        option={option}
        value={value}
        onValueChange={onValueChange}
      />
      <span className="provider-option-footer">
        {option.locked ? <em>Locked</em> : null}
        {!isDefault && !option.locked ? (
          <button
            type="button"
            data-testid={`provider-option-reset-${option.id}`}
            onClick={(event) => {
              event.preventDefault();
              onReset(option);
            }}
          >
            Reset
          </button>
        ) : null}
      </span>
    </label>
  );
}

interface ProviderOptionInputProps {
  option: ProviderOptionDefinition;
  value: ProviderOptionValue;
  onValueChange: (
    option: ProviderOptionDefinition,
    value: ProviderOptionValue,
  ) => void;
}

function ProviderOptionInput({
  option,
  value,
  onValueChange,
}: ProviderOptionInputProps) {
  const sharedProps = {
    "data-testid": `provider-option-input-${option.id}`,
    disabled: option.locked,
  };

  if (option.type === "boolean") {
    return (
      <input
        {...sharedProps}
        type="checkbox"
        checked={Boolean(value)}
        onChange={(event) => onValueChange(option, event.currentTarget.checked)}
      />
    );
  }

  if (option.type === "select") {
    return (
      <select
        {...sharedProps}
        value={String(value)}
        onChange={(event) => onValueChange(option, event.currentTarget.value)}
      >
        {(option.choices ?? []).map((choice) => (
          <option key={choice.value} value={choice.value}>
            {choice.label}
          </option>
        ))}
      </select>
    );
  }

  return (
    <input
      {...sharedProps}
      type={option.type === "secret" ? "password" : option.type}
      min={option.min}
      max={option.max}
      step={option.step}
      value={String(value)}
      onInput={(event) => onValueChange(option, event.currentTarget.value)}
      onChange={(event) => onValueChange(option, event.currentTarget.value)}
      spellCheck={false}
    />
  );
}

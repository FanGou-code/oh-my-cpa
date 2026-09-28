import { hasMessage, type TFunc } from './index';

/**
 * The console's name for an Agent capability.
 *
 * The registry's own name and description are written for the model: an identifier and an English
 * sentence about arguments and limits. The operator reads a localized title and a sentence about
 * what the call does to their deployment instead. A capability the console has no copy for - one
 * added on the server before the console caught up - reads as its identifier and the registry's
 * description, which are still the truth.
 */
export function capabilityTitle(name: string, t: TFunc): string {
  const key = `agent.capability.${name}`;
  return hasMessage(key) ? t(key) : name;
}

export function capabilityDescription(name: string, fallback: string, t: TFunc): string {
  const key = `agent.capability.${name}.description`;
  return hasMessage(key) ? t(key) : fallback;
}

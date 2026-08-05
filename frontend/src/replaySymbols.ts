export const replayRoleSymbols = [
  {
    role: "carry",
    mark: "AD",
    label: "Carry",
    description: "Primary sustained damage; usually fragile and highly dependent on positioning."
  },
  {
    role: "support",
    mark: "+",
    label: "Support",
    description: "Protects allies and helps create safer movement, vision, and engagement options."
  },
  {
    role: "jungler",
    mark: "JG",
    label: "Jungler",
    description: "A roaming flanker who can approach from routes outside the main lane."
  },
  {
    role: "mage",
    mark: "AP",
    label: "Mage",
    description: "Applies long-range spell damage and pressure around important areas."
  },
  {
    role: "tank",
    mark: "T",
    label: "Tank",
    description: "A durable front line that absorbs pressure and blocks access to allies."
  },
  {
    role: "assassin",
    mark: "A",
    label: "Assassin",
    description: "A burst threat that punishes isolated or low-health characters."
  }
] as const;

export const roleMarks: Record<string, string> = Object.fromEntries(
  replayRoleSymbols.map(({ role, mark }) => [role, mark])
);

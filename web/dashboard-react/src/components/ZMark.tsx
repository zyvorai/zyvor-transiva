// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// The Zyvor "Z", shared with the other Zyvor products (copied from the Netra dashboard).
//   "logomark": the bare orange Z, for wordmarks and navigation.
//   "tile":     the white Z on the orange gradient tile, for badges and the login page.
type ZMarkProps = { variant?: "logomark" | "tile"; size?: number };

export default function ZMark({ variant = "logomark", size = 22 }: ZMarkProps) {
  const file = variant === "tile" ? "zyvor-mark.svg" : "zyvor-logomark.svg";
  return (
    <img
      src={`${import.meta.env.BASE_URL}${file}`}
      alt=""
      aria-hidden="true"
      width={size}
      height={size}
      className="z-mark"
    />
  );
}

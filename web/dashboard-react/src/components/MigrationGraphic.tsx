// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { Cloud, ArrowRight, Check } from "lucide-react";
import ZMark from "./ZMark";
export function MigrationGraphic() {
  return (
    <div
      className="migration-art"
      aria-label="Migration pipeline: source, Transiva, destination"
      role="img"
    >
      <div className="art-orbit orbit-one" />
      <div className="art-orbit orbit-two" />
      <div className="art-node source">
        <Cloud size={34} />
        <span>Your infrastructure</span>
        <small>vSphere · Nutanix</small>
      </div>
      <ArrowRight className="art-arrow" aria-hidden="true" />
      <div className="art-core">
        <ZMark variant="tile" size={48} />
        <b>Transiva</b>
        <small>Move with clarity.</small>
      </div>
      <ArrowRight className="art-arrow" aria-hidden="true" />
      <div className="art-node target">
        <Check size={34} />
        <span>Your next chapter</span>
        <small>Portable VM artifacts</small>
      </div>
    </div>
  );
}

"use client";

import { IOS_APP_STORE_URL } from "@openagents/shared/constants";
import { useState } from "react";
import { usePlatform } from "../hooks/useOS";
import { StoreBadgeButton, StoreBadgeLink } from "./StoreBadge";
import { StoreQRDialog } from "./StoreQRDialog";

// Picks the install path by device. On the phone the badge is the install —
// one tap into the listing. On a desktop the same badge opens a QR, because
// the app cannot install on the machine the visitor is reading this on.
export function MobileAppCTA() {
  const { mobileOS } = usePlatform();
  const [open, setOpen] = useState(false);

  if (!IOS_APP_STORE_URL) {
    return <span className="text-sm text-muted-foreground">iOS app coming soon</span>;
  }

  if (mobileOS === "ios") {
    return (
      <StoreBadgeLink store="ios" href={IOS_APP_STORE_URL} />
    );
  }

  return (
    <>
      <StoreBadgeButton store="ios" onClick={() => setOpen(true)} />
      <StoreQRDialog
        platform="ios"
        url={IOS_APP_STORE_URL}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

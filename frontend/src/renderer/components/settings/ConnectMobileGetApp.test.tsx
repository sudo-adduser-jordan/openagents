import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { ANDROID_PLAY_STORE_URL, ConnectMobileGetApp, IOS_APP_STORE_URL } from "./ConnectMobileGetApp";

test("does not link retired mobile store listings before provisioning", () => {
	render(<ConnectMobileGetApp />);

	expect(IOS_APP_STORE_URL).toBeNull();
	expect(ANDROID_PLAY_STORE_URL).toBeNull();
	expect(screen.getByText("iOS")).toBeInTheDocument();
	expect(screen.getByText("Android")).toBeInTheDocument();
	expect(screen.getAllByText("The Open Agents store listing is being provisioned.")).toHaveLength(2);
	expect(screen.getAllByRole("button", { name: "Coming soon" })).toHaveLength(2);
	expect(screen.getAllByRole("button", { name: "Coming soon" }).every((button) => (button as HTMLButtonElement).disabled)).toBe(true);
});

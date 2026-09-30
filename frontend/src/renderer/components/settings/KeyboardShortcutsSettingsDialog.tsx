import { ShortcutBindingsEditor } from "./ShortcutBindingsEditor";

export function KeyboardShortcutsSettingsDialog({
	open,
	onOpenChange,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	return <ShortcutBindingsEditor mode="dialog" open={open} onOpenChange={onOpenChange} />;
}
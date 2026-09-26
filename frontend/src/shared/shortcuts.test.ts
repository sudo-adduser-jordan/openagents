import { describe, expect, it } from "vitest";
import {
	APP_SHORTCUTS,
	matchesAppShortcut,
	matchesFocusTerminalShortcut,
	matchesKeyboardShortcutsHelpShortcut,
	matchesNextTabShortcut,
	matchesNextSessionShortcut,
	matchesNewSessionShortcut,
	matchesNewShellTerminalShortcut,
	matchesOpenSettingsShortcut,
	matchesPreviousSessionShortcut,
	matchesPreviousTabShortcut,
	defaultShortcutBindings,
	matchesShortcutBinding,
	shortcutBindingValidationError,
	type ShortcutChord,
} from "./shortcuts";

function chord(overrides: Partial<ShortcutChord> & { key: string }): ShortcutChord {
	return { ctrl: false, meta: false, shift: false, alt: false, ...overrides };
}

describe("matchesNewSessionShortcut", () => {
	it("matches Ctrl+Shift+N on Windows/Linux", () => {
		expect(matchesNewSessionShortcut(chord({ key: "N", ctrl: true, shift: true }), false)).toBe(true);
	});

	it("does not match plain Ctrl+N on Windows/Linux (reserved for the terminal)", () => {
		expect(matchesNewSessionShortcut(chord({ key: "n", ctrl: true }), false)).toBe(false);
	});

	it("ignores other keys and extra modifiers", () => {
		expect(matchesNewSessionShortcut(chord({ key: "n", ctrl: true, shift: true, alt: true }), false)).toBe(false);
		expect(matchesNewSessionShortcut(chord({ key: "n", ctrl: true, shift: true, meta: true }), false)).toBe(false);
	});
});

describe("matchesNewShellTerminalShortcut", () => {
	it("matches Ctrl+T on Windows/Linux", () => {
		expect(matchesNewShellTerminalShortcut(chord({ key: "T", ctrl: true }), false)).toBe(true);
	});

	it("rejects extra modifiers", () => {
		expect(matchesNewShellTerminalShortcut(chord({ key: "`", ctrl: true }), false)).toBe(false);
		expect(matchesNewShellTerminalShortcut(chord({ key: "t", ctrl: true, shift: true }), false)).toBe(false);
	});
});

describe("matchesKeyboardShortcutsHelpShortcut", () => {
	it("matches Ctrl+/ on Windows/Linux", () => {
		expect(matchesKeyboardShortcutsHelpShortcut(chord({ key: "/", ctrl: true }), false)).toBe(true);
	});

	it("rejects extra modifiers", () => {
		expect(matchesKeyboardShortcutsHelpShortcut(chord({ key: "/", ctrl: true, shift: true }), false)).toBe(false);
		expect(matchesKeyboardShortcutsHelpShortcut(chord({ key: "?", ctrl: true }), false)).toBe(false);
	});
});

describe("additional application shortcuts", () => {
	it("matches settings and rejects extra modifiers", () => {
		expect(matchesOpenSettingsShortcut(chord({ key: ",", ctrl: true }), false)).toBe(true);
		expect(matchesOpenSettingsShortcut(chord({ key: ",", ctrl: true, shift: true }), false)).toBe(false);
	});

	it("matches previous and next session", () => {
		expect(matchesPreviousSessionShortcut(chord({ key: "PageUp", ctrl: true }), false)).toBe(true);
		expect(matchesNextSessionShortcut(chord({ key: "PageDown", ctrl: true }), false)).toBe(true);
		expect(matchesNextSessionShortcut(chord({ key: "Down", ctrl: true, alt: true }), false)).toBe(false);
		expect(matchesNextSessionShortcut(chord({ key: "Down", ctrl: true }), false)).toBe(false);
	});

	it("matches Ctrl+Tab and Ctrl+Shift+Tab", () => {
		expect(matchesNextTabShortcut(chord({ key: "Tab", ctrl: true }), false)).toBe(true);
		expect(matchesPreviousTabShortcut(chord({ key: "Tab", ctrl: true, shift: true }), false)).toBe(true);
		expect(matchesNextTabShortcut(chord({ key: "Tab", ctrl: true, shift: true }), false)).toBe(false);
		expect(matchesPreviousTabShortcut(chord({ key: "Tab", ctrl: true }), false)).toBe(false);
	});

	it("matches focus terminal and rejects extra modifiers", () => {
		expect(matchesFocusTerminalShortcut(chord({ key: "t", ctrl: true, shift: true }), false)).toBe(true);
		expect(matchesFocusTerminalShortcut(chord({ key: "t", ctrl: true, shift: true, alt: true }), false)).toBe(false);
	});

	it("matches close terminal", () => {
		expect(matchesAppShortcut("close-shell-terminal", chord({ key: "w", ctrl: true }), false)).toBe(true);
	});
});

describe("shortcut catalog", () => {
	it("provides runtime defaults for every shortcut", () => {
		for (const shortcut of APP_SHORTCUTS) {
			expect(defaultShortcutBindings(shortcut.id, false).length).toBeGreaterThan(0);
		}
	});

	it("uses a user override instead of the default binding", () => {
		const overrides = {
			"focus-terminal": [chord({ key: "j", ctrl: true })],
		};

		expect(matchesAppShortcut("focus-terminal", chord({ key: "j", ctrl: true }), false, overrides)).toBe(true);
		expect(matchesAppShortcut("focus-terminal", chord({ key: "t", ctrl: true, shift: true }), false, overrides)).toBe(
			false,
		);
	});
});

describe("shortcut binding matching and validation", () => {
	it("matches either the logical key or physical code when both are available", () => {
		const candidate = chord({ key: "`", code: "Backquote", ctrl: true });

		expect(matchesShortcutBinding(chord({ key: "`", code: "IntlBackslash", ctrl: true }), candidate)).toBe(true);
		expect(matchesShortcutBinding(chord({ key: "§", code: "Backquote", ctrl: true }), candidate)).toBe(true);
	});

	it("requires a modifier and reserves terminal-critical control chords", () => {
		expect(shortcutBindingValidationError(chord({ key: "F6" }), false)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "c", ctrl: true }), false)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "v", ctrl: true, shift: true }), false)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "d", ctrl: true }), false)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "j", ctrl: true }), false)).toBeNull();
	});

	it("reserves common platform window and editing chords", () => {
		expect(shortcutBindingValidationError(chord({ key: "q", meta: true }), true)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "F4", alt: true }), false)).not.toBeNull();
		expect(shortcutBindingValidationError(chord({ key: "j", meta: true }), true)).toBeNull();
	});
});

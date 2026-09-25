import { describe, expect, it } from "vitest";
import { formatManagerStartupError } from "./manager-startup-error";

describe("formatManagerStartupError", () => {
	it("replaces unresolved child remote errors with setup guidance", () => {
		const message =
			`Project added, but manager did not start: spawn workspace3-1: workspace: gitworktree: resolve workspace repo "test" base: workspace: default branch is unresolved: could not resolve remote "origin" HEAD for repository "/tmp/workspace3/test" (git -C /tmp/workspace3/test ls-remote --symref -- origin HEAD: exit status 128: remote: Repository not found. fatal: repository 'https://github.com/neversettle17-101/test.git/' not found); configure this repository's primary remote and cached HEAD (for example, git -C "/tmp/workspace3/test" remote set-head <remote> <branch>) and retry (DEFAULT_BRANCH_UNRESOLVED)`;

		expect(formatManagerStartupError(message)).toBe(
			'Project added, but manager did not start. The child repository "test" still needs its remote repository set up at https://github.com/neversettle17-101/test.git. Create or fix that remote, then retry starting the manager.',
		);
	});

	it("leaves unrelated spawn errors unchanged", () => {
		expect(formatManagerStartupError("Project added, but manager did not start: branch is already checked out"))
			.toBe("Project added, but manager did not start: branch is already checked out");
	});

	it.each([
		['resolve workspace repo "__root__" base: repository has no remote or Open Agents-recorded default', "workspace root repository"],
		['resolve workspace repo "api" base: remote did not advertise a symbolic HEAD', 'child repository "api"'],
		['repository has multiple remotes and no primary remote', "project repository"],
	])("preserves the actual default-branch failure: %s", (detail, label) => {
		const message = `${detail} (DEFAULT_BRANCH_UNRESOLVED)`;
		const result = formatManagerStartupError(message);
		expect(result).toContain(`default branch for the ${label}`);
		if (label.startsWith("child")) {
			expect(result).toContain("this child's default branch");
			expect(result).not.toContain("Project Settings");
		} else {
			expect(result).toContain("Project Settings");
		}
		expect(result).toContain(`Details: ${message}`);
		expect(result).not.toContain("still needs its remote repository set up");
	});
});

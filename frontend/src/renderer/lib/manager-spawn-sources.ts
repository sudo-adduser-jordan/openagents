export const MANAGER_SPAWN_SOURCES = [
	"board",
	"restore_dialog",
	"topbar",
	"sidebar",
	"project_add",
	"project_clone",
	"settings",
	"restart",
	"command_palette",
] as const;

export type ManagerSpawnSource = (typeof MANAGER_SPAWN_SOURCES)[number];

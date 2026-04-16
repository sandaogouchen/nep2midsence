package types

// PatternPageObjectTS identifies the TypeScript-specific Page Object pattern,
// where test code imports from .page.ts files or /pages/ directories.
// This pattern indicates the migration should convert TS Page Object classes
// to midscene Page objects with ai* intent calls replacing nep calls.
const PatternPageObjectTS PatternType = "page_object_ts"

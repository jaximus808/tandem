/* Starter content for the zero-tabs welcome page (roadmap item 10). Instead of
   one-click templates, the welcome page now leads with what you can ask your
   agent to build — each capability doubles as a copyable example prompt. */

export interface Capability {
  emoji: string;
  /** What kind of thing the agent can build. */
  title: string;
  /** One line on what it's good for. */
  blurb: string;
  /** A ready-to-copy prompt that produces this. */
  prompt: string;
}

// The breadth of the canvas — "stuff it can do." Each card is also a prompt you
// can copy and paste to your agent to try it.
export const CAPABILITIES: Capability[] = [
  {
    emoji: "🗂️",
    title: "Task board",
    blurb: "Epics and tickets your agents propose, you approve, and they work in parallel.",
    prompt: "Break the auth refactor into an epic with one ticket per file it touches, then wait for my approval.",
  },
  {
    emoji: "🧭",
    title: "Roadmaps",
    blurb: "Kanban goals and tasks you can drag around.",
    prompt: "Outline a product roadmap with goals grouped into Now, Next, and Later.",
  },
  {
    emoji: "📝",
    title: "Notes",
    blurb: "Free-form markdown docs alongside everything else.",
    prompt: "Write up the design decisions from this session as a notes tab.",
  },
];

// A few cross-cutting prompts that mix document types — "things to try" that show
// the agent building a whole workspace, not just one tab.
export const STARTER_PROMPTS: string[] = [
  "Read SPEC.md and propose an epic with one ticket per section.",
  "Sketch a launch plan as a roadmap with tasks I can check off.",
  "Turn my rough notes into key decisions and a list of action items.",
];

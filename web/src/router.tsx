import { createBrowserRouter } from "react-router";
import { PublicLayout } from "@/layouts/PublicLayout";
import {
  AboutPage,
  ArchivePage,
  HomePage,
  NotFoundPage,
  NotePage,
  PublicSearchPage,
  TagPage,
  TagsPage,
} from "@/routes/public/pages";
import { LoginPage } from "@/routes/app/LoginPage";
import { PageSpinner } from "@/components/ui";

// The workspace (editor, CodeMirror) is a separate chunk so blog readers
// don't download it.
const lazy = <K extends string>(load: () => Promise<Record<K, React.ComponentType>>, name: K) => ({
  lazy: async () => ({ Component: (await load())[name] }),
});

export const router = createBrowserRouter([
  {
    element: <PublicLayout />,
    children: [
      { index: true, element: <HomePage /> },
      { path: "n/:slug", element: <NotePage /> },
      { path: "tags", element: <TagsPage /> },
      { path: "tags/:name", element: <TagPage /> },
      { path: "archive", element: <ArchivePage /> },
      { path: "about", element: <AboutPage /> },
      { path: "search", element: <PublicSearchPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
  { path: "login", element: <LoginPage /> },
  {
    path: "app",
    HydrateFallback: PageSpinner,
    ...lazy(() => import("@/layouts/WorkspaceLayout"), "RequireAuth"),
    children: [
      {
        ...lazy(() => import("@/layouts/WorkspaceLayout"), "WorkspaceLayout"),
        children: [
          { index: true, ...lazy(() => import("@/routes/app/DashboardPage"), "DashboardPage") },
          {
            path: "notes",
            ...lazy(() => import("@/routes/app/NotesLayout"), "NotesLayout"),
            children: [
              { index: true, ...lazy(() => import("@/routes/app/NotesLayout"), "NoNoteSelected") },
              { path: ":id", ...lazy(() => import("@/routes/app/NoteEditorPage"), "NoteEditorPage") },
            ],
          },
          { path: "search", ...lazy(() => import("@/routes/app/SearchPage"), "SearchPage") },
          { path: "settings", ...lazy(() => import("@/routes/app/SettingsPage"), "SettingsPage") },
        ],
      },
    ],
  },
]);

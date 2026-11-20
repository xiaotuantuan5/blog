import { useMutation } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useInvalidateNotes } from "@/lib/queries";

export const UNTITLED = "无标题笔记";

/** Creates an empty note (optionally in a notebook) and opens it. */
export function useCreateNote() {
  const navigate = useNavigate();
  const [sp] = useSearchParams();
  const invalidate = useInvalidateNotes();
  return useMutation({
    mutationFn: (notebookId: number | null) =>
      api.createNote({ title: UNTITLED, slug: "", notebookId, contentMd: "", summary: "", visibility: "private", tags: [] }),
    onSuccess: (note) => {
      invalidate();
      const keep = new URLSearchParams(sp);
      keep.delete("q");
      const qs = keep.toString();
      navigate(`/app/notes/${note.id}${qs ? `?${qs}` : ""}`, { state: { focusTitle: true } });
    },
    onError: (e) => toast.error(e.message),
  });
}

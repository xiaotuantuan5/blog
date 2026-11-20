import { useMemo, useState, useEffect } from "react";
import { Link, useParams, useSearchParams, useNavigate } from "react-router";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, FileQuestion, PenLine, Search as SearchIcon } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { keys, useMe, useSite } from "@/lib/queries";
import { formatDate, useTitle } from "@/lib/utils";
import { NoteContent } from "@/components/NoteContent";
import { Toc } from "@/components/Toc";
import { Pager, PostList } from "@/components/PostList";
import { Empty, ErrorText, Input, PageSpinner } from "@/components/ui";

function usePage() {
  const [sp, setSp] = useSearchParams();
  const page = Math.max(1, Number(sp.get("page")) || 1);
  const setPage = (p: number) => {
    const next = new URLSearchParams(sp);
    if (p <= 1) next.delete("page");
    else next.set("page", String(p));
    setSp(next);
    window.scrollTo({ top: 0 });
  };
  return [page, setPage] as const;
}

export function HomePage() {
  const { data: site } = useSite();
  const [page, setPage] = usePage();
  const q = useQuery({
    queryKey: [...keys.public, "notes", page],
    queryFn: () => api.publicNotes({ page }),
    placeholderData: keepPreviousData,
  });
  useTitle(site?.siteTitle);

  return (
    <div className="mx-auto max-w-3xl">
      {site && (
        <section className="mb-10">
          <div className="flex items-center gap-4">
            {site.avatar && <img src={site.avatar} alt="" className="h-14 w-14 rounded-full object-cover" />}
            <div>
              <h1 className="text-2xl font-bold">{site.siteTitle}</h1>
              {site.siteDescription && <p className="mt-1 text-fg-muted">{site.siteDescription}</p>}
            </div>
          </div>
        </section>
      )}
      {q.isPending ? (
        <PageSpinner />
      ) : q.error ? (
        <ErrorText error={q.error} />
      ) : q.data.items.length === 0 ? (
        <Empty title="还没有公开的文章">在工作台中把笔记设为“公开”后会显示在这里。</Empty>
      ) : (
        <>
          <PostList items={q.data.items} />
          <Pager page={page} total={q.data.total} pageSize={q.data.pageSize} onPage={setPage} />
        </>
      )}
    </div>
  );
}

export function NotePage() {
  const { slug = "" } = useParams();
  const { data: site } = useSite();
  const { data: me } = useMe();
  const q = useQuery({
    queryKey: [...keys.public, "note", slug],
    queryFn: () => api.publicNote(slug),
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 2,
  });
  useTitle(q.data ? `${q.data.title} - ${site?.siteTitle ?? ""}` : undefined);

  // Scroll to the heading in the URL hash once content is rendered.
  useEffect(() => {
    if (q.data && location.hash) {
      document.getElementById(decodeURIComponent(location.hash.slice(1)))?.scrollIntoView();
    }
  }, [q.data]);

  if (q.isPending) return <PageSpinner />;
  if (q.error) {
    if (q.error instanceof ApiError && q.error.status === 404) return <NotFoundPage />;
    return <ErrorText error={q.error} />;
  }
  const n = q.data;

  return (
    <div className="lg:grid lg:grid-cols-[minmax(0,1fr)_14rem] lg:gap-10">
      <article className="min-w-0">
        <header className="mb-8">
          <h1 className="text-3xl font-bold leading-tight">{n.title}</h1>
          <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-fg-subtle">
            <time dateTime={n.publishedAt ?? undefined}>{formatDate(n.publishedAt)}</time>
            {n.updatedAt && formatDate(n.updatedAt) !== formatDate(n.publishedAt) && (
              <span>更新于 {formatDate(n.updatedAt)}</span>
            )}
            {n.tags.map((t) => (
              <Link key={t} to={`/tags/${encodeURIComponent(t)}`} className="hover:text-primary">
                #{t}
              </Link>
            ))}
            {me?.loggedIn && (
              <Link to={`/app/notes/${n.id}`} className="inline-flex items-center gap-1 hover:text-primary">
                <PenLine className="h-3.5 w-3.5" /> 编辑
              </Link>
            )}
          </div>
        </header>
        <NoteContent html={n.contentHtml} />
        {(n.prev || n.next) && (
          <nav className="mt-14 grid gap-3 border-t border-border pt-6 text-sm sm:grid-cols-2">
            {n.prev ? (
              <Link to={`/n/${n.prev.slug}`} className="group rounded-lg border border-border p-3 hover:border-primary">
                <span className="flex items-center gap-1 text-xs text-fg-subtle">
                  <ArrowLeft className="h-3 w-3" /> 上一篇
                </span>
                <span className="mt-1 block font-medium group-hover:text-primary">{n.prev.title}</span>
              </Link>
            ) : (
              <span />
            )}
            {n.next && (
              <Link to={`/n/${n.next.slug}`} className="group rounded-lg border border-border p-3 text-right hover:border-primary">
                <span className="flex items-center justify-end gap-1 text-xs text-fg-subtle">
                  下一篇 <ArrowRight className="h-3 w-3" />
                </span>
                <span className="mt-1 block font-medium group-hover:text-primary">{n.next.title}</span>
              </Link>
            )}
          </nav>
        )}
      </article>
      <aside className="hidden lg:block">
        <div className="sticky top-20 max-h-[calc(100vh-6rem)] overflow-y-auto thin-scroll">
          <Toc toc={n.toc} />
        </div>
      </aside>
    </div>
  );
}

export function TagsPage() {
  const { data: site } = useSite();
  const q = useQuery({ queryKey: [...keys.public, "tags"], queryFn: api.publicTags });
  useTitle(`标签 - ${site?.siteTitle ?? ""}`);
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-2xl font-bold">标签</h1>
      {q.isPending ? (
        <PageSpinner />
      ) : q.error ? (
        <ErrorText error={q.error} />
      ) : q.data.length === 0 ? (
        <Empty title="还没有标签" />
      ) : (
        <div className="flex flex-wrap gap-2.5">
          {q.data.map((t) => (
            <Link
              key={t.id}
              to={`/tags/${encodeURIComponent(t.name)}`}
              className="rounded-md border border-border px-3 py-1.5 text-sm hover:border-primary hover:text-primary"
            >
              {t.name} <span className="text-fg-subtle">{t.count}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export function TagPage() {
  const { name = "" } = useParams();
  const { data: site } = useSite();
  const [page, setPage] = usePage();
  const q = useQuery({
    queryKey: [...keys.public, "notes", "tag", name, page],
    queryFn: () => api.publicNotes({ tag: name, page }),
    placeholderData: keepPreviousData,
  });
  useTitle(`#${name} - ${site?.siteTitle ?? ""}`);
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-2xl font-bold">#{name}</h1>
      <p className="mb-8 text-sm text-fg-subtle">{q.data ? `共 ${q.data.total} 篇` : ""}</p>
      {q.isPending ? (
        <PageSpinner />
      ) : q.error ? (
        <ErrorText error={q.error} />
      ) : q.data.items.length === 0 ? (
        <Empty title="这个标签下没有公开文章" />
      ) : (
        <>
          <PostList items={q.data.items} />
          <Pager page={page} total={q.data.total} pageSize={q.data.pageSize} onPage={setPage} />
        </>
      )}
    </div>
  );
}

export function ArchivePage() {
  const { data: site } = useSite();
  const q = useQuery({ queryKey: [...keys.public, "archive"], queryFn: api.archive });
  useTitle(`归档 - ${site?.siteTitle ?? ""}`);

  const groups = useMemo(() => {
    const years = new Map<number, typeof q.data>();
    for (const it of q.data ?? []) {
      const y = new Date(it.publishedAt).getFullYear();
      if (!years.has(y)) years.set(y, []);
      years.get(y)!.push(it);
    }
    return [...years.entries()];
  }, [q.data]);

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-2xl font-bold">归档</h1>
      <p className="mb-8 text-sm text-fg-subtle">{q.data ? `共 ${q.data.length} 篇` : ""}</p>
      {q.isPending ? (
        <PageSpinner />
      ) : q.error ? (
        <ErrorText error={q.error} />
      ) : groups.length === 0 ? (
        <Empty title="还没有公开的文章" />
      ) : (
        <div className="space-y-10">
          {groups.map(([year, items]) => (
            <section key={year}>
              <h2 className="mb-3 text-lg font-semibold">
                {year} <span className="text-sm font-normal text-fg-subtle">{items!.length} 篇</span>
              </h2>
              <ul className="space-y-2">
                {items!.map((it) => (
                  <li key={it.slug} className="flex items-baseline gap-4">
                    <time className="w-14 shrink-0 font-mono text-xs text-fg-subtle">
                      {new Date(it.publishedAt).toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" })}
                    </time>
                    <Link to={`/n/${it.slug}`} className="hover:text-primary">
                      {it.title}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

export function AboutPage() {
  const { data: site, isPending } = useSite();
  useTitle(`关于 - ${site?.siteTitle ?? ""}`);
  if (isPending) return <PageSpinner />;
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-8 text-2xl font-bold">关于</h1>
      {site?.aboutHtml ? (
        <NoteContent html={site.aboutHtml} />
      ) : (
        <Empty title="还没有填写关于页面">登录后可以在“设置”中编辑。</Empty>
      )}
    </div>
  );
}

export function PublicSearchPage() {
  const [sp, setSp] = useSearchParams();
  const navigate = useNavigate();
  const urlQ = sp.get("q") ?? "";
  const [input, setInput] = useState(urlQ);
  const { data: site } = useSite();
  const [page, setPage] = usePage();
  useTitle(`搜索 - ${site?.siteTitle ?? ""}`);

  const q = useQuery({
    queryKey: [...keys.public, "search", urlQ, page],
    queryFn: () => api.publicNotes({ q: urlQ, page }),
    enabled: urlQ.trim() !== "",
    placeholderData: keepPreviousData,
  });

  return (
    <div className="mx-auto max-w-3xl">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const next = new URLSearchParams();
          if (input.trim()) next.set("q", input.trim());
          setSp(next);
        }}
        className="relative mb-8"
      >
        <SearchIcon className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-subtle" />
        <Input
          autoFocus
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="搜索文章，回车确认"
          className="h-11 pl-9 text-base"
          onKeyDown={(e) => e.key === "Escape" && navigate(-1)}
        />
      </form>
      {!urlQ ? null : q.isPending ? (
        <PageSpinner />
      ) : q.error ? (
        <ErrorText error={q.error} />
      ) : q.data.items.length === 0 ? (
        <Empty icon={<SearchIcon className="h-8 w-8" />} title={`没有找到“${urlQ}”相关的文章`} />
      ) : (
        <>
          <p className="mb-6 text-sm text-fg-subtle">找到 {q.data.total} 篇</p>
          <PostList items={q.data.items} />
          <Pager page={page} total={q.data.total} pageSize={q.data.pageSize} onPage={setPage} />
        </>
      )}
    </div>
  );
}

export function NotFoundPage() {
  useTitle("页面不存在");
  return (
    <Empty icon={<FileQuestion className="h-10 w-10" />} title="页面不存在">
      <Link to="/" className="text-primary hover:underline">
        返回首页
      </Link>
    </Empty>
  );
}

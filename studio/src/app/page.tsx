"use client";
import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { StudioHeader } from "../components/StudioHeader";
import { RecordsView } from "../components/RecordsView";
import { OverviewView } from "../components/OverviewView";
import { RecordSheet } from "../components/RecordSheet";
import { NewRecordDialog } from "../components/NewRecordDialog";
import { KeyAnalyzerDialog } from "../components/KeyAnalyzerDialog";
import { Button } from "../components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from "../components/ui/alert-dialog";
import { UniversalRecord } from "../types";
import { useStudioData } from "../hooks/useStudioData";

export default function StudioDashboard() {
  const data = useStudioData();
  const [view, setView] = useState<"records" | "overview">("records");
  const [selected, setSelected] = useState<UniversalRecord | null>(null);
  const [newOpen, setNewOpen] = useState(false);
  const [analyzerOpen, setAnalyzerOpen] = useState(false);
  const [deleteKey, setDeleteKey] = useState<string | null>(null);
  const [notice, setNotice] = useState<{
    message: string;
    error: boolean;
  } | null>(null);
  const showNotice = (message: string, error = false) =>
    setNotice({ message, error });
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 5000);
    return () => clearTimeout(timer);
  }, [notice]);
  const remove = async () => {
    if (!deleteKey) return;
    try {
      await data.remove(deleteKey);
      setSelected(null);
      showNotice("Record deleted.");
    } catch (e) {
      showNotice((e as Error).message, true);
    } finally {
      setDeleteKey(null);
    }
  };
  return (
    <div className="min-h-screen bg-background text-foreground">
      <StudioHeader
        view={view}
        setView={setView}
        user={data.user}
        busy={data.loading}
        onRefresh={() => void data.refresh()}
      />
      <main className="mx-auto max-w-[1440px] px-4 py-8 md:px-8 md:py-10">
        {view === "records" ? (
          <RecordsView
            records={data.records}
            loading={data.loading}
            loadingMore={data.loadingMore}
            hasMore={data.hasMore}
            error={data.error}
            canWrite={data.user?.role === "admin"}
            onRetry={() => void data.refresh()}
            onLoadMore={() => void data.loadMore()}
            onSelect={setSelected}
            onNew={() => setNewOpen(true)}
            onLookup={data.lookup}
            onNotice={showNotice}
          />
        ) : (
          <OverviewView
            stats={data.stats}
            statsError={data.statsError}
            recordsError={data.error}
            user={data.user}
            loaded={data.records.length}
            hasMore={data.hasMore}
            updatedAt={data.updatedAt}
            onRetry={() => void data.refresh()}
            onAnalyze={() => setAnalyzerOpen(true)}
          />
        )}
      </main>
      <RecordSheet
        record={selected}
        canWrite={data.user?.role === "admin"}
        onClose={() => setSelected(null)}
        onDelete={setDeleteKey}
        onNotice={showNotice}
      />
      <NewRecordDialog
        open={newOpen}
        onClose={() => setNewOpen(false)}
        onSave={data.save}
        onLookup={data.lookup}
        onNotice={showNotice}
      />
      <KeyAnalyzerDialog
        open={analyzerOpen}
        onClose={() => setAnalyzerOpen(false)}
      />
      <AlertDialog
        open={deleteKey !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteKey(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogTitle className="text-lg font-semibold">
            Delete this record?
          </AlertDialogTitle>
          <AlertDialogDescription className="mt-2 text-sm text-muted-foreground">
            Key <span className="font-mono">{deleteKey}</span> and its value
            will be permanently removed.
          </AlertDialogDescription>
          <div className="mt-6 flex justify-end gap-2">
            <AlertDialogCancel asChild>
              <Button variant="outline">Cancel</Button>
            </AlertDialogCancel>
            <AlertDialogAction asChild>
              <Button variant="destructive" onClick={() => void remove()}>
                Delete record
              </Button>
            </AlertDialogAction>
          </div>
        </AlertDialogContent>
      </AlertDialog>
      {notice && (
        <div
          role={notice.error ? "alert" : "status"}
          className={`fixed bottom-5 right-5 z-[70] flex max-w-sm items-center gap-3 rounded-lg border bg-card px-4 py-3 text-sm shadow-lg ${notice.error ? "border-destructive text-destructive" : ""}`}
        >
          <span>{notice.message}</span>
          <button aria-label="Dismiss message" onClick={() => setNotice(null)}>
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}

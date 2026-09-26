"use client";
import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { StudioHeader } from "../components/StudioHeader";
import { RecordsView } from "../components/RecordsView";
import { OverviewView } from "../components/OverviewView";
import { NewRecordDialog } from "../components/NewRecordDialog";
import { KeyAnalyzerDialog } from "../components/KeyAnalyzerDialog";
import { DeleteUserDialog } from "../components/DeleteUserDialog";
import { EditRecordPanel } from "../components/EditRecordPanel";
import { Dialog, DialogContent } from "../components/ui/dialog";
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
  const [deleteUserHash, setDeleteUserHash] = useState<number | null>(null);
  const [editingRecord, setEditingRecord] = useState<UniversalRecord | null>(null);
  const [notice, setNotice] = useState<{ message: string; error: boolean } | null>(null);

  const showNotice = (message: string, error = false) => setNotice({ message, error });

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

  const removeUserCascade = async () => {
    if (deleteUserHash === null) return;
    try {
      const res = await data.removeUser(deleteUserHash);
      setSelected(null);
      showNotice(`User deleted: ${res.deleted} associated records removed.`);
    } catch (e) {
      showNotice((e as Error).message, true);
    } finally {
      setDeleteUserHash(null);
    }
  };

  return (
    <div className="min-h-screen bg-background text-foreground">
      <StudioHeader view={view} setView={setView} user={data.user} busy={data.loading} onRefresh={() => void data.refresh()} />
      {data.user?.role === "viewer" && (
        <div className="border-b bg-amber-50 dark:bg-amber-950/20 px-4 py-2 text-center text-sm text-amber-800 dark:text-amber-200">
          You have view-only access — contact an admin to make changes.
        </div>
      )}
      <main className="mx-auto max-w-[1600px] px-4 py-8 md:px-8 md:py-10">
        {view === "records" ? (
          <RecordsView
            records={data.records}
            selected={selected}
            loading={data.loading}
            loadingMore={data.loadingMore}
            hasMore={data.hasMore}
            error={data.error}
            canWrite={data.user?.role === "admin"}
            onRetry={() => void data.refresh()}
            onLoadMore={() => void data.loadMore()}
            onSelect={setSelected}
            onDelete={setDeleteKey}
            onDeleteUser={(hash) => setDeleteUserHash(hash)}
            onEdit={(rec) => setEditingRecord(rec)}
            onNew={() => setNewOpen(true)}
            onLookup={data.lookup}
            onSearch={data.search}
            partitionCounts={data.stats?.partitionCounts}
            onLoadPartition={data.loadPartition}
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
      <NewRecordDialog open={newOpen} onClose={() => setNewOpen(false)} onSave={data.save} onLookup={data.lookup} onNotice={showNotice} />
      <KeyAnalyzerDialog open={analyzerOpen} onClose={() => setAnalyzerOpen(false)} />
      <DeleteUserDialog open={deleteUserHash !== null} userHash={deleteUserHash} onConfirm={() => void removeUserCascade()} onClose={() => setDeleteUserHash(null)} />
      <Dialog open={editingRecord !== null} onOpenChange={(open) => { if (!open) setEditingRecord(null); }}>
        <DialogContent className="max-w-2xl p-0">
          {editingRecord && (
            <EditRecordPanel
              record={editingRecord}
              onSave={async (key, val) => {
                await data.update(key, val);
                setEditingRecord(null);
                const updated = await data.lookup(key);
                setSelected(updated);
              }}
              onCancel={() => setEditingRecord(null)}
              onNotice={showNotice}
            />
          )}
        </DialogContent>
      </Dialog>
      <AlertDialog open={deleteKey !== null} onOpenChange={(open) => { if (!open) setDeleteKey(null); }}>
        <AlertDialogContent>
          <AlertDialogTitle className="text-lg font-semibold">Delete this record?</AlertDialogTitle>
          <AlertDialogDescription className="mt-2 text-sm text-muted-foreground">
            Key <span className="font-mono">{deleteKey}</span> and its value will be permanently removed.
          </AlertDialogDescription>
          <div className="mt-6 flex justify-end gap-2">
            <AlertDialogCancel asChild><Button variant="outline">Cancel</Button></AlertDialogCancel>
            <AlertDialogAction asChild><Button variant="destructive" onClick={() => void remove()}>Delete record</Button></AlertDialogAction>
          </div>
        </AlertDialogContent>
      </AlertDialog>
      {notice && (
        <div role={notice.error ? "alert" : "status"} className={`fixed bottom-5 right-5 z-[70] flex max-w-sm items-center gap-3 rounded-lg border bg-card px-4 py-3 text-sm shadow-lg ${notice.error ? "border-destructive text-destructive" : ""}`}>
          <span>{notice.message}</span>
          <button aria-label="Dismiss message" onClick={() => setNotice(null)}><X size={15} /></button>
        </div>
      )}
    </div>
  );
}

"use client";
import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { StudioSidebar, StudioView } from "../components/StudioSidebar";
import { RecordsView } from "../components/RecordsView";
import { OverviewView } from "../components/OverviewView";
import { QueryView } from "../components/views/QueryView";
import { IndexesView } from "../components/views/IndexesView";
import { ReplicationView } from "../components/views/ReplicationView";
import { BackupsView } from "../components/views/BackupsView";
import { KeysView } from "../components/views/KeysView";
import { MetricsView } from "../components/views/MetricsView";
import { LogsView } from "../components/views/LogsView";
import { SettingsView } from "../components/views/SettingsView";
import { NewRecordDialog } from "../components/NewRecordDialog";
import { KeyAnalyzerDialog } from "../components/KeyAnalyzerDialog";
import { DeleteUserDialog } from "../components/DeleteUserDialog";
import { EditRecordPanel } from "../components/EditRecordPanel";
import { ApiTokensDialog } from "../components/ApiTokensDialog";
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
  const [view, setView] = useState<StudioView>("explorer");
  const [selected, setSelected] = useState<UniversalRecord | null>(null);
  const [newOpen, setNewOpen] = useState(false);
  const [tokensOpen, setTokensOpen] = useState(false);
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

  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const key = params.get("key");
    if (key) {
      void data.lookup(key).then((rec) => {
        if (rec) setSelected(rec);
      }).catch(() => {});
    }
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (selected) {
      url.searchParams.set("key", selected.keyStr);
    } else {
      url.searchParams.delete("key");
    }
    window.history.replaceState({}, "", url.toString());
  }, [selected]);

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

  const viewProps = { stats: data.stats, user: data.user, onNotice: showNotice, onRefresh: () => void data.refresh() };

  return (
    <div className="flex min-h-dvh flex-col bg-background text-foreground md:h-dvh md:flex-row md:overflow-hidden">
      <StudioSidebar view={view} setView={setView} user={data.user} busy={data.loading} onRefresh={() => void data.refresh()} />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col md:overflow-y-auto">
      {data.user?.role === "viewer" && (
        <div className="border-b border-warning/30 bg-warning/10 px-4 py-2 text-center text-sm text-warning">
          You have view-only access — contact an admin to make changes.
        </div>
      )}
      <main className={view === "explorer"
        ? "flex min-h-0 w-full flex-1 flex-col px-4 py-4 md:px-8 md:py-6"
        : "w-full max-w-[1400px] px-4 py-6 md:px-8 md:py-8"}>
        {view === "explorer" && (
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
            onScanRange={data.scanRange}
            partitionCounts={data.stats?.partitionCounts}
            onLoadPartition={data.loadPartition}
            onNotice={showNotice}
            onOpenTokens={() => setTokensOpen(true)}
          />
        )}
        {view === "overview" && (
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
        {view === "query" && <QueryView {...viewProps} />}
        {view === "indexes" && <IndexesView {...viewProps} />}
        {view === "replication" && <ReplicationView {...viewProps} />}
        {view === "backups" && <BackupsView {...viewProps} />}
        {view === "keys" && <KeysView {...viewProps} />}
        {view === "metrics" && <MetricsView {...viewProps} />}
        {view === "logs" && <LogsView {...viewProps} />}
        {view === "settings" && <SettingsView {...viewProps} />}
      </main>
      </div>
      <NewRecordDialog open={newOpen} onClose={() => setNewOpen(false)} onSave={data.save} onLookup={data.lookup} onNotice={showNotice} />
      <ApiTokensDialog open={tokensOpen} onClose={() => setTokensOpen(false)} onNotice={showNotice} />
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

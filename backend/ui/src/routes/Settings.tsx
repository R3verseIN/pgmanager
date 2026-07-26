import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, Loader2, Save, ScrollText, Lock, Unlock, Download, RefreshCw, AlertTriangle } from "lucide-react";
import {
  fetchPgBouncerDatabases,
  togglePgBouncerDatabase,
  fetchPgBouncerConfig,
  updatePgBouncerConfig,
  fetchSettings,
  updateSettings,
  fetchSSLStatus,
  generateSSLCerts,
  uploadSSLCerts,
  downloadCACert,
  deleteSSLCerts,
  togglePgBouncerSSL,
} from "../api/client";
import type { PgBouncerDatabase, PgBouncerConfig } from "../api/client";
import { DatabaseAccessRow } from "../components/DatabaseAccessRow";
import { Select, SelectTrigger, SelectContent, SelectItem } from "../components/ui/select";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Button } from "../components/ui/button";
import { Switch } from "../components/ui/switch";
import { Badge } from "../components/ui/badge";

export default function Settings() {
  const queryClient = useQueryClient();

  // --- Database Access ---
  const { data: databases, isLoading } = useQuery({
    queryKey: ["pgbouncer-databases"],
    queryFn: fetchPgBouncerDatabases,
  });

  const toggleMutation = useMutation({
    mutationFn: ({
      databaseName,
      allowed,
    }: {
      databaseName: string;
      allowed: boolean;
    }) => togglePgBouncerDatabase(databaseName, allowed),
    onMutate: async ({ databaseName, allowed }) => {
      await queryClient.cancelQueries({ queryKey: ["pgbouncer-databases"] });
      const previous = queryClient.getQueryData<PgBouncerDatabase[]>([
        "pgbouncer-databases",
      ]);
      queryClient.setQueryData<PgBouncerDatabase[]>(
        ["pgbouncer-databases"],
        (old) =>
          old?.map((db) =>
            db.databaseName === databaseName ? { ...db, allowed } : db
          )
      );
      return { previous };
    },
    onError: (_err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(["pgbouncer-databases"], context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["pgbouncer-databases"] });
    },
  });

  // --- Connection Pool Config ---
  const { data: config, isLoading: configLoading } = useQuery({
    queryKey: ["pgbouncer-config"],
    queryFn: fetchPgBouncerConfig,
  });

  const [localConfig, setLocalConfig] = useState<PgBouncerConfig>({
    poolMode: "transaction",
    defaultPoolSize: 20,
    maxClientConn: 100,
  });

  useEffect(() => {
    if (config) {
      setLocalConfig(config);
    }
  }, [config]);

  const configMutation = useMutation({
    mutationFn: (newConfig: PgBouncerConfig) =>
      updatePgBouncerConfig(newConfig),
    onSuccess: (data) => {
      queryClient.setQueryData<PgBouncerConfig>(["pgbouncer-config"], data);
    },
  });

  // --- Log Retention ---
  const { data: settings, isLoading: settingsLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettings,
  });

  const [retentionDays, setRetentionDays] = useState(90);
  const [originalRetentionDays, setOriginalRetentionDays] = useState(90);

  useEffect(() => {
    if (settings?.audit_log_retention_days !== undefined) {
      const val = parseInt(settings.audit_log_retention_days) || 0;
      setRetentionDays(val);
      setOriginalRetentionDays(val);
    }
  }, [settings]);

  const retentionMutation = useMutation({
    mutationFn: (days: number) =>
      updateSettings("audit_log_retention_days", days.toString()),
    onSuccess: () => {
      setOriginalRetentionDays(retentionDays);
      queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
  });

  // --- SSL / TLS ---
  const { data: sslStatus, isLoading: sslLoading } = useQuery({
    queryKey: ["ssl-status"],
    queryFn: fetchSSLStatus,
  });

  const [showGenerateForm, setShowGenerateForm] = useState(false);
  const [showUploadForm, setShowUploadForm] = useState(false);
  const [generateCN, setGenerateCN] = useState("pgmanager-server");
  const [generateDays, setGenerateDays] = useState(1825);
  const uploadCertRef = useRef<HTMLInputElement>(null);
  const uploadKeyRef = useRef<HTMLInputElement>(null);
  const uploadCARef = useRef<HTMLInputElement>(null);

  const generateMutation = useMutation({
    mutationFn: generateSSLCerts,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ssl-status"] });
      setShowGenerateForm(false);
    },
  });

  const uploadMutation = useMutation({
    mutationFn: (formData: FormData) => uploadSSLCerts(formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ssl-status"] });
      setShowUploadForm(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteSSLCerts,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ssl-status"] });
    },
  });

  const pgbouncerSSLMutation = useMutation({
    mutationFn: togglePgBouncerSSL,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ssl-status"] });
    },
  });

  const handleUpload = () => {
    const certFile = uploadCertRef.current?.files?.[0];
    const keyFile = uploadKeyRef.current?.files?.[0];
    if (!certFile || !keyFile) return;

    const formData = new FormData();
    formData.append("server_cert", certFile);
    formData.append("server_key", keyFile);
    const caFile = uploadCARef.current?.files?.[0];
    if (caFile) {
      formData.append("ca_cert", caFile);
    }
    uploadMutation.mutate(formData);
  };

  const handleDownloadCA = async () => {
    try {
      const blob = await downloadCACert();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "root.crt";
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // Error handled by API layer
    }
  };

  const hasChanges =
    config &&
    (localConfig.poolMode !== config.poolMode ||
      localConfig.defaultPoolSize !== config.defaultPoolSize ||
      localConfig.maxClientConn !== config.maxClientConn);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          Settings
        </h1>
        <p className="text-sm text-ink-muted">
          Manage PgBouncer and SSL configuration
        </p>
      </div>

      {/* SSL / TLS */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="mb-4 flex items-center gap-3">
          {sslStatus?.enabled ? (
            <Lock className="size-5 text-green-500" />
          ) : (
            <Unlock className="size-5 text-ink-muted" />
          )}
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-medium text-foreground">
                SSL / TLS
              </h2>
              {sslStatus?.enabled ? (
                <Badge variant="success">Active</Badge>
              ) : (
                <Badge variant="secondary">Inactive</Badge>
              )}
              {sslStatus?.pendingRestart && (
                <span className="inline-flex items-center rounded-full border border-amber-300 bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-700">
                  Restart Required
                </span>
              )}
            </div>
            <p className="text-xs text-ink-muted">
              Secure external PostgreSQL connections with SSL/TLS encryption
            </p>
          </div>
        </div>

        {sslLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : (
          <div className="space-y-4">
            {/* Status */}
            {sslStatus?.hasCerts && (
              <div className="rounded-md bg-surface-2 p-3 text-xs space-y-1">
                <div className="flex justify-between">
                  <span className="text-ink-muted">Issuer</span>
                  <span className="text-foreground">{sslStatus.issuer}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-ink-muted">Expires</span>
                  <span className="text-foreground">
                    {sslStatus.expiry ? new Date(sslStatus.expiry).toLocaleDateString() : "N/A"}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-ink-muted">Type</span>
                  <span className="text-foreground">
                    {sslStatus.selfSigned ? "Self-signed CA" : "Custom certificate"}
                  </span>
                </div>
              </div>
            )}

            {/* Actions */}
            <div className="flex flex-wrap gap-2">
              {!sslStatus?.hasCerts && (
                <>
                  <Button
                    size="sm"
                    variant="default"
                    disabled={generateMutation.isPending}
                    onClick={() => {
                      setShowGenerateForm(!showGenerateForm);
                      setShowUploadForm(false);
                    }}
                  >
                    {generateMutation.isPending ? (
                      <Loader2 className="mr-1 size-4 animate-spin" />
                    ) : (
                      <RefreshCw className="mr-1 size-4" />
                    )}
                    Generate Certificates
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setShowUploadForm(!showUploadForm);
                      setShowGenerateForm(false);
                    }}
                  >
                    Upload Custom
                  </Button>
                </>
              )}
              {sslStatus?.hasCerts && (
                <>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={handleDownloadCA}
                  >
                    <Download className="mr-1 size-4" />
                    Download CA
                  </Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={deleteMutation.isPending}
                    onClick={() => {
                      if (confirm("Disable SSL and remove certificates? This will require clients to reconnect without SSL.")) {
                        deleteMutation.mutate();
                      }
                    }}
                  >
                    {deleteMutation.isPending ? (
                      <Loader2 className="mr-1 size-4 animate-spin" />
                    ) : (
                      <AlertTriangle className="mr-1 size-4" />
                    )}
                    Disable SSL
                  </Button>
                </>
              )}
            </div>

            {/* Generate Form */}
            {showGenerateForm && (
              <div className="rounded-md border border-hairline bg-surface-2 p-4 space-y-3">
                <div className="space-y-2">
                  <Label className="text-sm text-ink-muted">Common Name</Label>
                  <Input
                    value={generateCN}
                    onChange={(e) => setGenerateCN(e.target.value)}
                    placeholder="e.g., pg.example.com or server IP"
                    className="border-hairline bg-surface-1"
                  />
                  <p className="text-xs text-ink-muted">
                    The hostname or IP clients will use to connect
                  </p>
                </div>
                <div className="space-y-2">
                  <Label className="text-sm text-ink-muted">Valid for (days)</Label>
                  <Input
                    type="number"
                    min={30}
                    max={3650}
                    value={generateDays}
                    onChange={(e) => setGenerateDays(parseInt(e.target.value) || 1825)}
                    className="border-hairline bg-surface-1"
                  />
                </div>
                <div className="flex justify-end gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setShowGenerateForm(false)}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    variant="default"
                    disabled={generateMutation.isPending}
                    onClick={() =>
                      generateMutation.mutate({
                        commonName: generateCN,
                        validityDays: generateDays,
                      })
                    }
                  >
                    {generateMutation.isPending && (
                      <Loader2 className="mr-1 size-4 animate-spin" />
                    )}
                    Generate & Enable
                  </Button>
                </div>
              </div>
            )}

            {/* Upload Form */}
            {showUploadForm && (
              <div className="rounded-md border border-hairline bg-surface-2 p-4 space-y-3">
                <div className="space-y-2">
                  <Label className="text-sm text-ink-muted">Server Certificate (.crt)</Label>
                  <input
                    ref={uploadCertRef}
                    type="file"
                    accept=".crt,.pem,.cert"
                    className="block w-full text-sm text-ink-muted file:mr-4 file:rounded-md file:border-0 file:bg-surface-1 file:px-3 file:py-1.5 file:text-sm file:text-foreground hover:file:bg-surface-2"
                  />
                </div>
                <div className="space-y-2">
                  <Label className="text-sm text-ink-muted">Server Key (.key)</Label>
                  <input
                    ref={uploadKeyRef}
                    type="file"
                    accept=".key,.pem"
                    className="block w-full text-sm text-ink-muted file:mr-4 file:rounded-md file:border-0 file:bg-surface-1 file:px-3 file:py-1.5 file:text-sm file:text-foreground hover:file:bg-surface-2"
                  />
                </div>
                <div className="space-y-2">
                  <Label className="text-sm text-ink-muted">CA Certificate (optional)</Label>
                  <input
                    ref={uploadCARef}
                    type="file"
                    accept=".crt,.pem"
                    className="block w-full text-sm text-ink-muted file:mr-4 file:rounded-md file:border-0 file:bg-surface-1 file:px-3 file:py-1.5 file:text-sm file:text-foreground hover:file:bg-surface-2"
                  />
                </div>
                <div className="flex justify-end gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setShowUploadForm(false)}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    variant="default"
                    disabled={uploadMutation.isPending}
                    onClick={handleUpload}
                  >
                    {uploadMutation.isPending && (
                      <Loader2 className="mr-1 size-4 animate-spin" />
                    )}
                    Upload & Enable
                  </Button>
                </div>
              </div>
            )}

            {/* PgBouncer SSL Toggle */}
            {sslStatus?.hasCerts && (
              <div className="flex items-center justify-between rounded-md bg-surface-2 p-3">
                <div>
                  <p className="text-sm font-medium text-foreground">
                    PgBouncer Client TLS
                  </p>
                  <p className="text-xs text-ink-muted">
                    Accept SSL connections from clients through PgBouncer
                    {sslStatus.pendingRestart && (
                      <span className="ml-1 text-amber-500">
                        (restart pgbouncer container to apply)
                      </span>
                    )}
                  </p>
                </div>
                <Switch
                  checked={sslStatus.pgBouncerSSL}
                  disabled={pgbouncerSSLMutation.isPending}
                  onCheckedChange={(checked) =>
                    pgbouncerSSLMutation.mutate(checked)
                  }
                />
              </div>
            )}
          </div>
        )}
      </div>

      {/* Connection Pool */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="mb-4 flex items-center gap-3">
          <Loader2 className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              Connection Pool
            </h2>
            <p className="text-xs text-ink-muted">
              Configure PgBouncer connection pooling behavior
            </p>
          </div>
        </div>

        {configLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className="text-sm text-ink-muted">Pool Mode</Label>
              <Select
                value={localConfig.poolMode}
                onValueChange={(value) =>
                  setLocalConfig({ ...localConfig, poolMode: value })
                }
              >
                <SelectTrigger className="w-full border-hairline bg-surface-2">
                  <span>{localConfig.poolMode}</span>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="session">session</SelectItem>
                  <SelectItem value="transaction">transaction</SelectItem>
                  <SelectItem value="statement">statement</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-ink-muted">
                {localConfig.poolMode === "transaction"
                  ? "Best for most applications — connections released after transaction completes"
                  : localConfig.poolMode === "session"
                    ? "Connections held until client disconnects — safe but slower"
                    : "Connections released after each statement — fastest but breaks stateful features"}
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">Default Pool Size</Label>
                <Input
                  type="number"
                  min={1}
                  max={10000}
                  value={localConfig.defaultPoolSize}
                  onChange={(e) =>
                    setLocalConfig({
                      ...localConfig,
                      defaultPoolSize: parseInt(e.target.value) || 20,
                    })
                  }
                  className="border-hairline bg-surface-2"
                />
                <p className="text-xs text-ink-muted">
                  Max server connections per user/database pair
                </p>
              </div>

              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">Max Client Connections</Label>
                <Input
                  type="number"
                  min={1}
                  max={100000}
                  value={localConfig.maxClientConn}
                  onChange={(e) =>
                    setLocalConfig({
                      ...localConfig,
                      maxClientConn: parseInt(e.target.value) || 100,
                    })
                  }
                  className="border-hairline bg-surface-2"
                />
                <p className="text-xs text-ink-muted">
                  Total max connections PgBouncer accepts from clients
                </p>
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <Button
                disabled={!hasChanges || configMutation.isPending}
                onClick={() => configMutation.mutate(localConfig)}
                size="sm"
              >
                {configMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Save className="mr-1 size-4" />
                )}
                Save
              </Button>
            </div>
          </div>
        )}
      </div>

      {/* Database Access */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="mb-4 flex items-center gap-3">
          <Shield className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              PgBouncer Database Access
            </h2>
            <p className="text-xs text-ink-muted">
              Control which databases are accessible through PgBouncer (external
              connections)
            </p>
          </div>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : !databases || databases.length === 0 ? (
          <p className="py-4 text-sm text-ink-muted">No databases found</p>
        ) : (
          <div className="space-y-1">
            {databases.map((db: PgBouncerDatabase) => (
              <DatabaseAccessRow
                key={db.databaseName}
                databaseName={db.databaseName}
                allowed={db.allowed}
                disabled={toggleMutation.isPending}
                onToggle={(checked) =>
                  toggleMutation.mutate({
                    databaseName: db.databaseName,
                    allowed: checked,
                  })
                }
              />
            ))}
          </div>
        )}
      </div>

      {/* Log Retention */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="mb-4 flex items-center gap-3">
          <ScrollText className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              Log Retention
            </h2>
            <p className="text-xs text-ink-muted">
              Automatically delete old audit log entries
            </p>
          </div>
        </div>

        {settingsLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className="text-sm text-ink-muted">Retention (days)</Label>
              <Input
                type="number"
                min={0}
                max={3650}
                value={retentionDays}
                onChange={(e) =>
                  setRetentionDays(parseInt(e.target.value) || 0)
                }
                className="border-hairline bg-surface-2"
              />
              <p className="text-xs text-ink-muted">
                Delete audit logs older than N days. Set to 0 to keep forever.
                Cleanup runs daily.
              </p>
            </div>

            <div className="flex justify-end pt-2">
              <Button
                disabled={
                  retentionDays === originalRetentionDays ||
                  retentionMutation.isPending
                }
                onClick={() => retentionMutation.mutate(retentionDays)}
                size="sm"
              >
                {retentionMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Save className="mr-1 size-4" />
                )}
                Save
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

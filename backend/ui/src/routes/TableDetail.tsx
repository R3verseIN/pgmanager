import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import type { WhereCondition } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";

import { Button } from "../components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import DataTab from "../components/tabs/DataTab";
import StructureTab from "../components/tabs/StructureTab";
import InsertRowDialog from "../components/dialogs/InsertRowDialog";
import EditRowDialog from "../components/dialogs/EditRowDialog";
import DeleteRowConfirm from "../components/dialogs/DeleteRowConfirm";
import AddColumnDialog from "../components/dialogs/AddColumnDialog";

export default function TableDetail() {
  const { name: dbName, table } = useParams<{ name: string; table: string }>();
  const navigate = useNavigate();
  const { isAdmin, isDev } = useAuth();
  const [tab, setTab] = useState("data");

  const [page, setPage] = useState(0);
  const [sortCol, setSortCol] = useState("");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [insertOpen, setInsertOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editRow, setEditRow] = useState<unknown[] | null>(null);
  const [deleteWhere, setDeleteWhere] = useState<WhereCondition[] | null>(null);
  const [addColOpen, setAddColOpen] = useState(false);

  if (!dbName || !table) return null;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate(`/databases/${dbName}`)}>
          <ArrowLeft className="size-4" />
        </Button>
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-muted">
            <Link to={`/databases/${dbName}`} className="transition-colors hover:text-accent-blue">
              {dbName}
            </Link>
            <span>/</span>
          </div>
          <h1 className="text-xl font-(--font-display) tracking-tight">{table}</h1>
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="data">Data</TabsTrigger>
          <TabsTrigger value="structure">Structure</TabsTrigger>
        </TabsList>

        <TabsContent value="data">
          <DataTab
            dbName={dbName}
            table={table}
            page={page}
            setPage={setPage}
            sortCol={sortCol}
            setSortCol={setSortCol}
            sortDir={sortDir}
            setSortDir={setSortDir}
            canWrite={isAdmin || isDev}
            onInsert={() => setInsertOpen(true)}
            onEdit={(row) => {
              setEditRow(row);
              setEditOpen(true);
            }}
            onDelete={(where) => {
              setDeleteWhere(where);
            }}
          />
        </TabsContent>

        <TabsContent value="structure">
          <StructureTab
            dbName={dbName}
            table={table}
            canWrite={isAdmin || isDev}
            onAddColumn={() => setAddColOpen(true)}
          />
        </TabsContent>
      </Tabs>

      <InsertRowDialog
        dbName={dbName}
        table={table}
        open={insertOpen}
        onOpenChange={setInsertOpen}
      />
      <EditRowDialog
        dbName={dbName}
        table={table}
        open={editOpen}
        onOpenChange={setEditOpen}
        row={editRow}
      />
      <DeleteRowConfirm
        dbName={dbName}
        table={table}
        where={deleteWhere}
        onOpenChange={(open) => {
          if (!open) setDeleteWhere(null);
        }}
      />
      <AddColumnDialog
        dbName={dbName}
        table={table}
        open={addColOpen}
        onOpenChange={setAddColOpen}
      />
    </div>
  );
}

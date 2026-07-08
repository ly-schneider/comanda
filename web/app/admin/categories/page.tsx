"use client"
import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core"
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import { GripVertical } from "lucide-react"
import { useEffect, useState } from "react"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"

type Category = { id: string; name: string; isActive: boolean; position: number }

export default function AdminCategoriesPage() {
  const fetchAuth = useAuthorizedFetch()
  const [items, setItems] = useState<Category[]>([])
  const [name, setName] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [nameError, setNameError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  useEffect(() => {
    void reload()
  }, [])

  async function reload() {
    try {
      const res = await fetchAuth(`/api/v1/categories`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = (await res.json()) as { items: Category[] }
      const sorted = (data.items || []).slice().sort((a, b) => a.position - b.position || a.name.localeCompare(b.name))
      setItems(sorted)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to load"
      setError(msg)
    }
  }

  async function patchCategory(id: string, body: Partial<Pick<Category, "name" | "isActive" | "position">>) {
    const csrf = getCSRFToken()
    return fetchAuth(`/api/v1/categories/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify(body),
    })
  }

  async function createCategory() {
    if (!name.trim()) {
      setNameError("Name ist erforderlich.")
      return
    }
    setNameError(null)
    const nextPosition = items.reduce((max, c) => Math.max(max, c.position), -1) + 1
    setCreating(true)
    try {
      const csrf = getCSRFToken()
      const res = await fetchAuth(`/api/v1/categories`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({ name: name.trim(), position: nextPosition }),
      })
      if (!res.ok) {
        setError(await readErrorMessage(res))
        return
      }
      setName("")
      await reload()
    } finally {
      setCreating(false)
    }
  }

  async function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const oldIndex = items.findIndex((c) => c.id === active.id)
    const newIndex = items.findIndex((c) => c.id === over.id)
    if (oldIndex < 0 || newIndex < 0) return

    const previous = items
    const reordered = arrayMove(items, oldIndex, newIndex).map((c, i) => ({ ...c, position: i }))
    setItems(reordered)

    const changed = reordered.filter((c) => previous.find((p) => p.id === c.id)?.position !== c.position)
    try {
      const results = await Promise.all(changed.map((c) => patchCategory(c.id, { position: c.position })))
      if (results.some((r) => !r.ok)) throw new Error("reorder_failed")
    } catch {
      setError("Reihenfolge konnte nicht gespeichert werden.")
      await reload()
    }
  }

  async function toggle(id: string, isActive: boolean) {
    const res = await patchCategory(id, { isActive })
    if (!res.ok) {
      setError(await readErrorMessage(res))
      return
    }
    await reload()
  }

  async function remove(id: string) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/categories/${id}`, {
      method: "DELETE",
      headers: { "X-CSRF": csrf || "" },
    })
    if (!res.ok) {
      setError(await readErrorMessage(res))
      return
    }
    await reload()
  }

  async function rename(id: string, newName: string) {
    const trimmed = newName.trim()
    if (!trimmed) return
    const res = await patchCategory(id, { name: trimmed })
    if (!res.ok) {
      setError(await readErrorMessage(res))
      return
    }
    await reload()
  }

  return (
    <div className="min-w-0 space-y-4">
      <h1 className="text-xl font-semibold">Kategorien</h1>
      {error && <div className="text-sm text-red-600">{error}</div>}

      <div className="flex items-start gap-2">
        <div className="space-y-1">
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              if (nameError) setNameError(null)
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                void createCategory()
              }
            }}
            placeholder="Neue Kategorie"
            className="h-8 w-64"
            maxLength={20}
            aria-invalid={!!nameError}
          />
          {nameError && <p className="text-destructive text-sm">{nameError}</p>}
        </div>
        <Button variant="outline" size="sm" className="h-8" disabled={creating} onClick={() => void createCategory()}>
          Erstellen
        </Button>
      </div>

      <TableContainer>
        <div
          className="overflow-x-auto overscroll-x-contain"
          role="region"
          aria-label="Categories table – scroll horizontally to reveal more columns"
          tabIndex={0}
        >
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <Table className="whitespace-nowrap">
              <TableHeader className="bg-card sticky top-0">
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead className="whitespace-nowrap">Name</TableHead>
                  <TableHead className="whitespace-nowrap">Status</TableHead>
                  <TableHead className="text-right whitespace-nowrap">Aktionen</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <SortableContext items={items.map((c) => c.id)} strategy={verticalListSortingStrategy}>
                  {items.map((c) => (
                    <CategoryRow
                      key={c.id}
                      category={c}
                      onNameChange={(newName) =>
                        setItems((prev) => prev.map((it) => (it.id === c.id ? { ...it, name: newName } : it)))
                      }
                      onRename={(newName) => void rename(c.id, newName)}
                      onToggle={(v) => void toggle(c.id, v)}
                      onRemove={() => void remove(c.id)}
                    />
                  ))}
                </SortableContext>
              </TableBody>
            </Table>
          </DndContext>
        </div>
      </TableContainer>
    </div>
  )
}

function CategoryRow({
  category,
  onNameChange,
  onRename,
  onToggle,
  onRemove,
}: {
  category: Category
  onNameChange: (name: string) => void
  onRename: (name: string) => void
  onToggle: (isActive: boolean) => void
  onRemove: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: category.id })
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    ...(isDragging ? { position: "relative", zIndex: 10 } : {}),
  }

  return (
    <TableRow ref={setNodeRef} style={style} className="even:bg-card odd:bg-muted/40">
      <TableCell className="w-10">
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground flex cursor-grab touch-none items-center active:cursor-grabbing"
          aria-label="Zeile verschieben"
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" />
        </button>
      </TableCell>
      <TableCell>
        <Input
          value={category.name}
          onChange={(e) => onNameChange(e.target.value)}
          onBlur={(e) => onRename(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              e.currentTarget.blur()
            }
          }}
          maxLength={20}
          className="h-7 w-40"
        />
      </TableCell>
      <TableCell>
        <label className="inline-flex items-center gap-2">
          <Switch checked={category.isActive} onCheckedChange={onToggle} />
          <span>{category.isActive ? "Aktiv" : "Inaktiv"}</span>
        </label>
      </TableCell>
      <TableCell className="text-right">
        <div className="inline-flex items-center gap-1">
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="ghost" size="sm" className="h-7 text-red-700">
                Löschen
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Kategorie löschen?</AlertDialogTitle>
                <AlertDialogDescription>
                  Diese Aktion kann nicht rückgängig gemacht werden. Kategorie "{category.name}" dauerhaft löschen?
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Abbrechen</AlertDialogCancel>
                <AlertDialogAction onClick={onRemove}>Löschen</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </TableCell>
    </TableRow>
  )
}

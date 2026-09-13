/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Code, Plus, Table, Trash2, X } from 'lucide-react'
import {
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'

import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type ModelMappingEditorProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

type MappingTarget = {
  key: string
  model: string
  retry: string
}

type MappingRow = {
  id: string
  from: string
  targets: MappingTarget[]
}

const DUPLICATE_MAPPING_SENTINEL = '{ "duplicate_source_models": '

function getDuplicateSources(rows: MappingRow[]): string[] {
  const seen = new Set<string>()
  const duplicates = new Set<string>()

  for (const row of rows) {
    const source = row.from.trim()
    if (!source) continue
    if (seen.has(source)) {
      duplicates.add(source)
    } else {
      seen.add(source)
    }
  }

  return [...duplicates]
}

// A single target without retries serializes as a plain string so legacy
// 1:1 mappings stay untouched; anything richer becomes an ordered queue.
function serializeTargets(targets: MappingTarget[]): string | unknown[] {
  const entries = targets
    .map((target) => ({
      model: target.model.trim(),
      retry: Number.parseInt(target.retry, 10) || 0,
    }))
    .filter((target) => target.model !== '')
  if (entries.length === 0) {
    return ''
  }
  if (entries.length === 1 && entries[0].retry <= 0) {
    return entries[0].model
  }
  return entries.map((entry) =>
    entry.retry > 0 ? { model: entry.model, retry: entry.retry } : entry.model
  )
}

export function ModelMappingEditor(props: ModelMappingEditorProps) {
  const { t } = useTranslation()
  const sourceListId = useId()
  const targetListId = useId()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<MappingRow[]>([])
  const [jsonValue, setJsonValue] = useState(props.value)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const nextIdRef = useRef(0)
  const duplicateSources = useMemo(() => getDuplicateSources(rows), [rows])

  const createId = (prefix: string) => {
    nextIdRef.current += 1
    return `${prefix}-${nextIdRef.current}`
  }

  const parseTargets = (value: unknown): MappingTarget[] => {
    const items = Array.isArray(value) ? value : [value]
    const targets: MappingTarget[] = []
    for (const item of items) {
      if (typeof item === 'string' && item.trim() !== '') {
        targets.push({ key: createId('target'), model: item, retry: '' })
      } else if (item && typeof item === 'object' && !Array.isArray(item)) {
        const record = item as Record<string, unknown>
        if (typeof record.model === 'string' && record.model.trim() !== '') {
          const retry =
            typeof record.retry === 'number' && record.retry > 0
              ? String(record.retry)
              : ''
          targets.push({
            key: createId('target'),
            model: record.model,
            retry,
          })
        }
      }
    }
    return targets
  }

  const parseJsonToRows = (json: string): boolean => {
    try {
      if (!json.trim()) {
        setRows([])
        setJsonError(null)
        return true
      }
      const parsed = JSON.parse(json)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        setJsonError(t('Model mapping must be a valid JSON object'))
        return false
      }
      const entries = Object.entries(parsed)
      const invalidValue = entries.find(
        ([, value]) =>
          typeof value !== 'string' &&
          !(Array.isArray(value) && value.length > 0) &&
          !(
            value &&
            typeof value === 'object' &&
            !Array.isArray(value) &&
            typeof (value as Record<string, unknown>).model === 'string'
          )
      )
      if (invalidValue) {
        setJsonError(
          t(
            'Model mapping must be a JSON object of strings or ordered model queues'
          )
        )
        return false
      }
      setRows((previousRows) => {
        const remainingRows = [...previousRows]
        return entries.map(([from, value]) => {
          const existingIndex = remainingRows.findIndex(
            (row) => row.from === from
          )
          const parsedTargets = parseTargets(value)
          if (existingIndex >= 0) {
            const [existing] = remainingRows.splice(existingIndex, 1)
            return {
              id: existing.id,
              from,
              targets:
                parsedTargets.length > 0
                  ? parsedTargets
                  : [{ key: createId('target'), model: '', retry: '' }],
            }
          }
          return {
            id: createId('mapping'),
            from,
            targets:
              parsedTargets.length > 0
                ? parsedTargets
                : [{ key: createId('target'), model: '', retry: '' }],
          }
        })
      })
      setJsonError(null)
      return true
    } catch {
      setJsonError(t('Model mapping must be valid JSON format'))
      return false
    }
  }

  const syncExternalValue = useEffectEvent(() => {
    setJsonValue(props.value)
    parseJsonToRows(props.value)
  })

  // Only replace the draft when the external value changes, not on language changes.
  useEffect(() => {
    syncExternalValue()
  }, [props.value])

  const convertRowsToJson = (updatedRows: MappingRow[]): string => {
    if (updatedRows.length === 0) {
      return ''
    }
    const obj: Record<string, unknown> = {}
    updatedRows.forEach((row) => {
      if (row.from.trim()) {
        obj[row.from.trim()] = serializeTargets(row.targets)
      }
    })
    return JSON.stringify(obj, null, 2)
  }

  const syncRows = (updatedRows: MappingRow[]) => {
    setRows(updatedRows)
    const duplicates = getDuplicateSources(updatedRows)
    if (duplicates.length > 0) {
      setJsonError(t('Duplicate source model mappings are not allowed'))
      setJsonValue(DUPLICATE_MAPPING_SENTINEL)
      props.onChange(DUPLICATE_MAPPING_SENTINEL)
      return
    }

    const json = convertRowsToJson(updatedRows)
    setJsonError(null)
    setJsonValue(json)
    props.onChange(json)
  }

  const handleAddRow = () => {
    const newRow: MappingRow = {
      id: createId('mapping'),
      from: '',
      targets: [{ key: createId('target'), model: '', retry: '' }],
    }
    syncRows([...rows, newRow])
  }

  const handleDeleteRow = (id: string) => {
    syncRows(rows.filter((row) => row.id !== id))
  }

  const handleRowChange = (id: string, field: 'from', newValue: string) => {
    const updatedRows = rows.map((row) =>
      row.id === id ? { ...row, [field]: newValue } : row
    )
    syncRows(updatedRows)
  }

  const handleTargetChange = (
    rowId: string,
    targetKey: string,
    field: 'model' | 'retry',
    newValue: string
  ) => {
    const updatedRows = rows.map((row) =>
      row.id === rowId
        ? {
            ...row,
            targets: row.targets.map((target) =>
              target.key === targetKey
                ? { ...target, [field]: newValue }
                : target
            ),
          }
        : row
    )
    syncRows(updatedRows)
  }

  const handleAddTarget = (rowId: string) => {
    const updatedRows = rows.map((row) =>
      row.id === rowId
        ? {
            ...row,
            targets: [
              ...row.targets,
              { key: createId('target'), model: '', retry: '' },
            ],
          }
        : row
    )
    syncRows(updatedRows)
  }

  const handleDeleteTarget = (rowId: string, targetKey: string) => {
    const updatedRows = rows.map((row) =>
      row.id === rowId
        ? { ...row, targets: row.targets.filter((t) => t.key !== targetKey) }
        : row
    )
    syncRows(updatedRows)
  }

  const handleJsonChange = (newJson: string) => {
    setJsonValue(newJson)
    props.onChange(newJson)
    parseJsonToRows(newJson)
  }

  const handleFillTemplate = () => {
    const template = JSON.stringify(
      {
        'gpt-3.5-turbo': 'gpt-3.5-turbo-0125',
        'gemini-2.5-pro': [
          'gemini-2.5-pro-06-05',
          { model: 'gemini-2.5-pro-free', retry: 1 },
        ],
      },
      null,
      2
    )
    setJsonValue(template)
    props.onChange(template)
    parseJsonToRows(template)
  }

  const handleModeChange = (nextMode: string) => {
    if (nextMode !== 'visual' && nextMode !== 'json') return
    if (nextMode === 'json') {
      const duplicates = getDuplicateSources(rows)
      if (duplicates.length === 0) {
        const json = convertRowsToJson(rows)
        setJsonValue(json)
        props.onChange(json)
      }
      setMode('json')
      return
    }
    parseJsonToRows(jsonValue)
    setMode('visual')
  }

  return (
    <div className='space-y-2'>
      <Tabs value={mode} onValueChange={handleModeChange} className='space-y-2'>
        <div className='flex items-center justify-between gap-3'>
          <TabsList>
            <TabsTrigger value='visual'>
              <Table className='h-4 w-4' aria-hidden='true' />
              {t('Visual')}
            </TabsTrigger>
            <TabsTrigger value='json'>
              <Code className='h-4 w-4' aria-hidden='true' />
              {t('JSON')}
            </TabsTrigger>
          </TabsList>
          <Button
            type='button'
            variant='link'
            size='sm'
            className='h-auto p-0'
            onClick={handleFillTemplate}
            disabled={props.disabled}
          >
            {t('Fill Template')}
          </Button>
        </div>

        {jsonError && (
          <Alert variant='destructive'>
            <AlertDescription>{jsonError}</AlertDescription>
          </Alert>
        )}

        {duplicateSources.length > 0 && (
          <Alert>
            <AlertDescription>
              {t('Duplicate source model(s): {{models}}', {
                models: duplicateSources.join(', '),
              })}
            </AlertDescription>
          </Alert>
        )}

        <TabsContent value='visual' className='space-y-2'>
          {rows.length > 0 ? (
            <div className='space-y-2'>
              <div className='grid grid-cols-[1fr_2fr_auto] gap-2 text-sm font-medium'>
                <div>{t('Request Model Name')}</div>
                <div>{t('Upstream Model Name')}</div>
                <div className='w-10' />
              </div>
              {rows.map((row) => (
                <div
                  key={row.id}
                  className='grid grid-cols-[1fr_2fr_auto] items-start gap-2'
                >
                  <Input
                    value={row.from}
                    onChange={(e) =>
                      handleRowChange(row.id, 'from', e.target.value)
                    }
                    placeholder='gpt-3.5-turbo'
                    disabled={props.disabled}
                    className='h-10'
                    list={sourceListId}
                  />
                  <div className='space-y-1.5'>
                    {row.targets.map((target) => (
                      <div
                        key={target.key}
                        className='flex items-center gap-1.5'
                      >
                        <Input
                          value={target.model}
                          onChange={(e) =>
                            handleTargetChange(
                              row.id,
                              target.key,
                              'model',
                              e.target.value
                            )
                          }
                          placeholder='gpt-3.5-turbo-0125'
                          disabled={props.disabled}
                          className='h-10 flex-1'
                          list={targetListId}
                        />
                        <Input
                          value={target.retry}
                          onChange={(e) =>
                            handleTargetChange(
                              row.id,
                              target.key,
                              'retry',
                              e.target.value
                            )
                          }
                          type='number'
                          min={0}
                          step={1}
                          placeholder='0'
                          disabled={props.disabled}
                          className='h-10 w-16'
                          aria-label={t('Retry count')}
                        />
                        {row.targets.length > 1 && (
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            onClick={() =>
                              handleDeleteTarget(row.id, target.key)
                            }
                            disabled={props.disabled}
                            className='h-9 w-9'
                            aria-label={t('Delete target')}
                          >
                            <X className='h-4 w-4' aria-hidden='true' />
                          </Button>
                        )}
                      </div>
                    ))}
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='h-7 px-2 text-xs'
                      onClick={() => handleAddTarget(row.id)}
                      disabled={props.disabled}
                    >
                      <Plus className='mr-1 h-3 w-3' aria-hidden='true' />
                      {t('Add Target')}
                    </Button>
                  </div>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    onClick={() => handleDeleteRow(row.id)}
                    disabled={props.disabled}
                    className='h-10 w-10'
                    aria-label={t('Delete mapping')}
                  >
                    <Trash2 className='h-4 w-4' aria-hidden='true' />
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed text-sm'>
              {t(
                'No model mappings configured. Click "Add Mapping" to get started.'
              )}
            </div>
          )}
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={handleAddRow}
            disabled={props.disabled}
            className='w-full'
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('Add Mapping')}
          </Button>
        </TabsContent>
        <TabsContent value='json' className='space-y-2'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'JSON keys are request model names; values are upstream model names.'
            )}
          </p>
          <JsonCodeEditor
            value={jsonValue}
            onChange={handleJsonChange}
            placeholder={t(
              '{"model": "upstream", "multi": ["upstream-1", {"model": "upstream-2", "retry": 1}]}'
            )}
            disabled={props.disabled}
            className={jsonError ? 'border-destructive' : undefined}
            aria-invalid={Boolean(jsonError)}
            ariaLabel={t('Model Mapping')}
          />
        </TabsContent>
      </Tabs>

      {props.sourceModelOptions && props.sourceModelOptions.length > 0 && (
        <datalist id={sourceListId}>
          {props.sourceModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
      {props.targetModelOptions && props.targetModelOptions.length > 0 && (
        <datalist id={targetListId}>
          {props.targetModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
    </div>
  )
}

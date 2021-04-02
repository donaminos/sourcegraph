import { replaceRange } from '../../util/strings'
import { FilterType } from './filters'
import { stringHuman } from './printer'
import { scanSearchQuery } from './scanner'
import { createLiteral, Filter } from './token'
import { filterExists } from './validate'

export function appendContextFilter(query: string, searchContextSpec: string | undefined): string {
    return !filterExists(query, FilterType.context) && searchContextSpec
        ? `context:${searchContextSpec} ${query}`
        : query
}

export const omitFilter = (query: string, filter: Filter): string => {
    let finalQuery = replaceRange(query, filter.range)
    if (filter.range.start === 0) {
        // Remove space at the start
        finalQuery = finalQuery.slice(1)
    }
    return finalQuery
}

/**
 * Updates all filters with the given value if they exist.
 * Appends a single filter at the top level of the query if none exist.
 * This function expects a valid query; if it is invalid it throws.
 */
export function updateFilters(query: string, field: string, value: string, negated = false): string {
    const range = { start: -1, end: -1 }
    const filter: Filter = {
        type: 'filter',
        range,
        field: createLiteral(field, range),
        value: createLiteral(value, range),
        negated,
    }
    const scanned = scanSearchQuery(query)
    if (scanned.type !== 'success') {
        throw new Error('Internal error: invariant broken: updateFilters must be called with a valid query')
    }
    let modified = false
    const result = scanned.term.map(token => {
        if (token.type === 'filter' && token.field.value.toLowerCase() === field) {
            modified = true
            return filter
        }
        return token
    })
    if (!modified) {
        scanned.term.push({ type: 'whitespace', range }, filter)
        return stringHuman(scanned.term)
    }
    return stringHuman(result)
}

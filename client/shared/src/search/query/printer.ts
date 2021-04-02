import { Token } from './token'

export const stringHuman = (tokens: Token[]): string => {
    const result: string[] = []
    for (const token of tokens) {
        switch (token.type) {
            case 'whitespace':
                result.push(' ')
                break
            case 'openingParen':
                result.push('(')
                break
            case 'closingParen':
                result.push(')')
                break
            case 'filter':
                // eslint-disable-next-line no-case-declarations
                let value = ''
                if (token.value) {
                    if (token.value.quoted) {
                        value = JSON.stringify(token.value.value)
                    } else {
                        value = token.value.value
                    }
                }
                result.push(`${token.field.value}:${value}`)
                break
            case 'keyword':
            case 'comment':
            case 'pattern':
            case 'literal':
                result.push(token.value)
                break
        }
    }
    return result.join('')
}

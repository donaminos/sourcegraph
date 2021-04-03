import CloudAlertIcon from 'mdi-react/CloudAlertIcon'
import CloudCheckIcon from 'mdi-react/CloudCheckIcon'
import CloudSyncIcon from 'mdi-react/CloudSyncIcon'
import CloudOffOutlineIcon from 'mdi-react/CloudOffOutlineIcon'
import CheckboxMarkedCircleIcon from 'mdi-react/CheckboxMarkedCircleIcon'
import InformationCircleIcon from 'mdi-react/InformationCircleIcon'
import React from 'react'
import { ButtonDropdown, DropdownMenu, DropdownToggle } from 'reactstrap'
import { Observable, Subscription, of } from 'rxjs'
import { catchError, map, repeatWhen, delay, distinctUntilChanged, switchMap } from 'rxjs/operators'
import { Link } from '../../../shared/src/components/Link'
import { CircleDashedIcon } from '../components/CircleDashedIcon'
import { dataOrThrowErrors, gql } from '../../../shared/src/graphql/graphql'
import { asError, ErrorLike, isErrorLike } from '../../../shared/src/util/errors'
import { requestGraphQL } from '../backend/graphql'
import classNames from 'classnames'
import { ErrorAlert } from '../components/alerts'
import * as H from 'history'
import { repeatUntil } from '../../../shared/src/util/rxjs/repeatUntil'
import { StatusMessagesResult } from '../graphql-operations'
import { queryExternalServices } from '../components/externalServices/backend'
import { isEqual } from 'lodash'

function fetchAllStatusMessages(): Observable<StatusMessagesResult['statusMessages']> {
    return requestGraphQL<StatusMessagesResult>(
        gql`
            query StatusMessages {
                statusMessages {
                    ...StatusMessageFields
                }
            }

            fragment StatusMessageFields on StatusMessage {
                type: __typename

                ... on CloningProgress {
                    message
                }

                ... on IndexingProgress {
                    message
                }

                ... on SyncError {
                    message
                }

                ... on ExternalServiceSyncError {
                    message
                    externalService {
                        id
                        displayName
                    }
                }
            }
        `
    ).pipe(
        map(dataOrThrowErrors),
        map(data => data.statusMessages)
    )
}

type EntryType = 'not-active' | 'progress' | 'warning' | 'success' | 'error'

interface StatusMessageEntryProps {
    text: string
    linkTo: string
    linkText: string
    entryType: EntryType
    linkOnClick: (event: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => void
    title?: string
}

function entryIcon(entryType: EntryType): JSX.Element {
    switch (entryType) {
        case 'error': {
            return <InformationCircleIcon className="icon-inline mr-2 text-danger" />
        }
        case 'warning':
            return <CloudAlertIcon className="icon-inline mr-2" />
        case 'success':
            return <CheckboxMarkedCircleIcon className="icon-inline mr-2 text-success" />
        case 'progress':
            return <CloudSyncIcon className="icon-inline mr-2" />
        case 'not-active':
            return <CircleDashedIcon className="icon-inline status-messages-nav-item__entry-off-icon mr-2" />
    }
}

const StatusMessagesNavItemEntry: React.FunctionComponent<StatusMessageEntryProps> = props => (
    <div key={props.text} className="status-messages-nav-item__entry">
        <p className="text-muted status-messages-nav-item__entry-sync">Code sync status</p>
        <h4>
            {entryIcon(props.entryType)}
            {props.title ? props.title : 'Your repositories'}
        </h4>
        {props.entryType === 'not-active' ? (
            <div className="status-messages-nav-item__entry-card status-messages-nav-item__entry-card--muted border-0">
                <p className="text-muted status-messages-nav-item__entry-message">{props.text}</p>
                <Link className="text-primary" to={props.linkTo} onClick={props.linkOnClick}>
                    {props.linkText}
                </Link>
            </div>
        ) : (
            <div
                className={classNames(
                    'status-messages-nav-item__entry-card',
                    `status-messages-nav-item__entry--border-${props.entryType}`,
                    'status-messages-nav-item__entry-card--active mt-0'
                )}
            >
                <p className="status-messages-nav-item__entry-message">{props.text}</p>
                <Link className="text-primary" to={props.linkTo} onClick={props.linkOnClick}>
                    {props.linkText}
                </Link>
            </div>
        )}
    </div>
)

interface Props {
    fetchMessages?: () => Observable<StatusMessagesResult['statusMessages']>
    isSiteAdmin: boolean
    history: H.History
    userCreatedAt: string
    userID: string
}

enum ExternalServiceNoActivityReasons {
    NO_CODEHOSTS = 'NO_CODEHOSTS',
    NO_REPOS = 'NO_REPOS',
}

type ExternalServiceNoActivityReason = keyof typeof ExternalServiceNoActivityReasons
type Message = StatusMessagesResult['statusMessages'] | ExternalServiceNoActivityReason
type MessageOrError = Message | ErrorLike

const isNoActivityReason = (status: MessageOrError): status is ExternalServiceNoActivityReason =>
    typeof status === 'string'

interface State {
    messagesOrError: MessageOrError
    isOpen: boolean
}

const REFRESH_INTERVAL_AFTER_ERROR_MS = 3000
const REFRESH_INTERVAL_MS = 10000

/**
 * Displays a status icon in the navbar reflecting the completion of backend
 * tasks such as repository cloning, and exposes a dropdown menu containing
 * more information on these tasks.
 */
export class StatusMessagesNavItem extends React.PureComponent<Props, State> {
    private subscriptions = new Subscription()

    public state: State = { isOpen: false, messagesOrError: [] }

    private toggleIsOpen = (): void => this.setState(previousState => ({ isOpen: !previousState.isOpen }))

    public componentDidMount(): void {
        this.subscriptions.add(
            queryExternalServices({
                namespace: this.props.userID,
                first: null,
                after: null,
            })
                .pipe(
                    switchMap(({ nodes: services }) => {
                        if (services.length === 0) {
                            return of(ExternalServiceNoActivityReasons.NO_CODEHOSTS)
                        }

                        if (
                            !services.some(service => service.repoCount !== 0) &&
                            services.every(service => service.lastSyncError == null && service.warning == null)
                        ) {
                            return of(ExternalServiceNoActivityReasons.NO_REPOS)
                        }

                        return (this.props.fetchMessages ?? fetchAllStatusMessages)()
                    }),
                    catchError(error => [asError(error) as ErrorLike]),
                    // Poll on REFRESH_INTERVAL_MS, or REFRESH_INTERVAL_AFTER_ERROR_MS if there is an error.
                    repeatUntil(messagesOrError => isErrorLike(messagesOrError), { delay: REFRESH_INTERVAL_MS }),
                    repeatWhen(completions => completions.pipe(delay(REFRESH_INTERVAL_AFTER_ERROR_MS))),
                    distinctUntilChanged((a, b) => isEqual(a, b))
                )
                .subscribe(messagesOrError => {
                    this.setState({ messagesOrError })
                })
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    // StatusMessageFields | ExternalServiceNoActivityReason | []
    private renderMessage(message: Message): JSX.Element | JSX.Element[] {
        // no status messages
        if (Array.isArray(message) && message.length === 0) {
            return (
                <StatusMessagesNavItemEntry
                    text="All repositories up to date"
                    linkTo="/site-admin/external-services"
                    linkText="Manage repositories"
                    linkOnClick={this.toggleIsOpen}
                    entryType="success"
                />
            )
        }

        // no code hosts or no repos
        if (isNoActivityReason(message)) {
            if (message === ExternalServiceNoActivityReasons.NO_REPOS) {
                return (
                    <StatusMessagesNavItemEntry
                        key={message}
                        text="Add repositories to start searching your code on Sourcegraph."
                        linkTo=""
                        linkText="Add repositories"
                        linkOnClick={this.toggleIsOpen}
                        entryType="not-active"
                    />
                )
            }
            return (
                <StatusMessagesNavItemEntry
                    key={message}
                    text="Connect with a code host to start adding your code to Sourcegraph."
                    linkTo=""
                    linkText="Connect with code host"
                    linkOnClick={this.toggleIsOpen}
                    entryType="not-active"
                />
            )
        }

        return message.map(message => {
            switch (message.type) {
                case 'CloningProgress':
                    return (
                        <StatusMessagesNavItemEntry
                            key={message.message}
                            text={message.message}
                            linkTo="/site-admin/external-services"
                            linkText="Configure synced repositories"
                            linkOnClick={this.toggleIsOpen}
                            entryType="progress"
                        />
                    )
                case 'IndexingProgress':
                    return (
                        <StatusMessagesNavItemEntry
                            key={message.message}
                            text={message.message}
                            linkTo="/site-admin/external-services"
                            linkText="Configure synced repositories"
                            linkOnClick={this.toggleIsOpen}
                            entryType="progress"
                        />
                    )
                case 'ExternalServiceSyncError':
                    return (
                        <StatusMessagesNavItemEntry
                            key={message.message}
                            text={message.message}
                            linkTo={`/site-admin/external-services/${message.externalService.id}`}
                            linkText={`Edit "${message.externalService.displayName}"`}
                            linkOnClick={this.toggleIsOpen}
                            entryType="error"
                        />
                    )
                case 'SyncError':
                    return (
                        <StatusMessagesNavItemEntry
                            key={message.message}
                            text={message.message}
                            linkTo="/site-admin/external-services"
                            linkText="Configure synced repositories"
                            linkOnClick={this.toggleIsOpen}
                            entryType="warning"
                        />
                    )
            }
        })
    }

    private renderIcon(): JSX.Element | null {
        if (isErrorLike(this.state.messagesOrError)) {
            return <CloudAlertIcon className="icon-inline" />
        }

        if (isNoActivityReason(this.state.messagesOrError)) {
            return <CloudOffOutlineIcon className="icon-inline" />
        }

        if (this.state.messagesOrError.some(({ type }) => type === 'ExternalServiceSyncError')) {
            return (
                <CloudAlertIcon
                    className="icon-inline"
                    data-tooltip={this.state.isOpen ? undefined : 'Syncing repositories failed!'}
                />
            )
        }
        if (this.state.messagesOrError.some(({ type }) => type === 'CloningProgress')) {
            return (
                <CloudSyncIcon
                    className="icon-inline"
                    data-tooltip={this.state.isOpen ? undefined : 'Cloning repositories...'}
                />
            )
        }
        return (
            <CloudCheckIcon
                className="icon-inline"
                data-tooltip={this.state.isOpen ? undefined : 'Repositories up to date'}
            />
        )
    }

    public render(): JSX.Element | null {
        return (
            <ButtonDropdown
                isOpen={this.state.isOpen}
                toggle={this.toggleIsOpen}
                className="nav-link py-0 px-0 percy-hide chromatic-ignore"
            >
                <DropdownToggle caret={false} className="btn btn-link" nav={true}>
                    {this.renderIcon()}
                </DropdownToggle>

                <DropdownMenu right={true} className="status-messages-nav-item__dropdown-menu p-0">
                    <div className="status-messages-nav-item__dropdown-menu-content">
                        {isErrorLike(this.state.messagesOrError) ? (
                            <ErrorAlert
                                className="status-messages-nav-item__entry"
                                prefix="Failed to load status messages"
                                error={this.state.messagesOrError}
                            />
                        ) : (
                            this.renderMessage(this.state.messagesOrError)
                        )}
                    </div>
                </DropdownMenu>
            </ButtonDropdown>
        )
    }
}

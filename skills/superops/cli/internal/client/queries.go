// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// GraphQL operations for the SuperOps MSP API (https://api.superops.ai/msp).
//
// SuperOps is NOT a Relay-style GraphQL API. Its list queries take a single
// `input: ListInfoInput!` argument carrying page/pageSize/condition/sort, and
// return a `<entity>List { <entities> ..., listInfo { totalCount hasMore } }`
// wrapper rather than a `{ nodes, pageInfo }` connection. To reuse the
// generated read-command + extraction plumbing unchanged, each list query:
//
//   - inlines the pagination into `input: { pageSize: $first, page: $page }`
//     so the generated scalar variables map (which sets "first"/"page") binds
//     directly, and
//   - aliases the entity array to `nodes:` so extractGraphQLConnection /
//     PaginatedQuery (which read `<field>.nodes`) work without change.
//
// listInfo.hasMore drives page-based pagination in client.PaginatedQuery.
//
// Get queries wrap the id in the entity's *IdentifierInput object
// (e.g. getTicket(input: { ticketId: $id })).
//
// VERIFY-ON-FIRST-USE: the inner entity-array field names below follow
// SuperOps' documented `get<Entity>List { <entityPlural> }` convention
// (confirmed from the docs for tickets and alerts). The higher-uncertainty
// ones (clientSites, clientUsers, clientContracts, worklogEntries, kbItems,
// itDocumentations, serviceItems) are best-effort from that convention; if a
// specific list returns empty against a live tenant, adjust the alias target
// here — it is a one-line fix per query. See README "Known Gaps".

package client

const AlertsListQuery = `query($first: Int, $page: Int) {
  getAlertList(input: {pageSize: $first, page: $page}) {
    nodes: alerts {
      id
      message
      description
      severity
      status
      createdTime
      resolvedTime
      asset
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const AssetsGetQuery = `query($id: ID!) {
  getAsset(input: {assetId: $id}) {
    assetId
    name
    assetClass
    hostName
    serialNumber
    manufacturer
    model
    platform
    platformCategory
    platformVersion
    publicIp
    loggedInUser
    status
    patchStatus
    agentVersion
    lastCommunicatedTime
    warrantyExpiryDate
    client
    site
  }
}`

const AssetsListQuery = `query($first: Int, $page: Int) {
  getAssetList(input: {pageSize: $first, page: $page}) {
    nodes: assets {
      assetId
      name
      assetClass
      hostName
      serialNumber
      manufacturer
      model
      platform
      platformCategory
      platformVersion
      publicIp
      loggedInUser
      status
      patchStatus
      agentVersion
      lastCommunicatedTime
      warrantyExpiryDate
      client
      site
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const ClientsGetQuery = `query($id: ID!) {
  getClient(input: {accountId: $id}) {
    accountId
    name
    stage
    status
    emailDomains
    accountManager
    primaryContact
    hqSite
  }
}`

const ClientsListQuery = `query($first: Int, $page: Int) {
  getClientList(input: {pageSize: $first, page: $page}) {
    nodes: clients {
      accountId
      name
      stage
      status
      emailDomains
      accountManager
      primaryContact
      hqSite
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const ContractsListQuery = `query($first: Int, $page: Int) {
  getClientContractList(input: {pageSize: $first, page: $page}) {
    nodes: clientContracts {
      contractId
      name
      contractType
      startDate
      endDate
      client
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const InvoicesGetQuery = `query($id: ID!) {
  getInvoice(input: {invoiceId: $id}) {
    invoiceId
    displayId
    invoiceDate
    dueDate
    statusEnum
    sentToClient
    totalAmount
    discountAmount
    paymentDate
    paymentMethod
    client
    site
  }
}`

const InvoicesListQuery = `query($first: Int, $page: Int) {
  getInvoiceList(input: {pageSize: $first, page: $page}) {
    nodes: invoices {
      invoiceId
      displayId
      invoiceDate
      dueDate
      statusEnum
      sentToClient
      totalAmount
      discountAmount
      paymentDate
      paymentMethod
      client
      site
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const ItDocsListQuery = `query($first: Int, $page: Int) {
  getItDocumentationList(input: {pageSize: $first, page: $page}) {
    nodes: itDocumentations {
      itDocumentationId
      name
      description
      type
      createdTime
      updatedTime
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const KbListQuery = `query($first: Int, $page: Int) {
  getKbItems(listInfo: {pageSize: $first, page: $page}) {
    nodes: kbItems {
      itemId
      title
      content
      status
      createdTime
      updatedTime
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const ServiceItemsListQuery = `query($first: Int, $page: Int) {
  getServiceItemList(input: {pageSize: $first, page: $page}) {
    nodes: serviceItems {
      itemId
      name
      description
      quantityType
      unitPrice
      afterHoursUnitPrice
      salesTaxEnabled
      category
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const SitesListQuery = `query($first: Int, $page: Int) {
  getClientSiteList(input: {pageSize: $first, page: $page}) {
    nodes: clientSites {
      id
      name
      line1
      city
      stateCode
      postalCode
      countryCode
      contactNumber
      timezoneCode
      hq
      client
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const TasksGetQuery = `query($id: ID!) {
  getTask(input: {taskId: $id}) {
    taskId
    displayId
    title
    description
    status
    estimatedTime
    scheduledStartDate
    dueDate
    overdue
    actualStartDate
    actualEndDate
    technician
    ticket
  }
}`

const TasksListQuery = `query($first: Int, $page: Int) {
  getTaskList(input: {pageSize: $first, page: $page}) {
    nodes: tasks {
      taskId
      displayId
      title
      description
      status
      estimatedTime
      scheduledStartDate
      dueDate
      overdue
      actualStartDate
      actualEndDate
      technician
      ticket
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const TechniciansListQuery = `query($first: Int, $page: Int) {
  getTechnicianList(input: {pageSize: $first, page: $page}) {
    nodes: technicians {
      userId
      name
      email
      contactNumber
      designation
      role
      team
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const TicketsGetQuery = `query($id: ID!) {
  getTicket(input: {ticketId: $id}) {
    ticketId
    displayId
    subject
    ticketType
    status
    priority
    client
    site
    requester
    technician
    techGroup
    sla
    createdTime
    updatedTime
    resolutionDueTime
    resolutionTime
    firstResponseDueTime
  }
}`

const TicketsListQuery = `query($first: Int, $page: Int) {
  getTicketList(input: {pageSize: $first, page: $page}) {
    nodes: tickets {
      ticketId
      displayId
      subject
      ticketType
      status
      priority
      client
      site
      requester
      technician
      techGroup
      sla
      createdTime
      updatedTime
      resolutionDueTime
      resolutionTime
      firstResponseDueTime
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const UsersListQuery = `query($first: Int, $page: Int) {
  getClientUserList(input: {pageSize: $first, page: $page}) {
    nodes: clientUsers {
      userId
      name
      firstName
      lastName
      email
      contactNumber
      role
      site
      client
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

const WorklogsListQuery = `query($first: Int, $page: Int) {
  getWorklogEntries(input: {pageSize: $first, page: $page}) {
    nodes: worklogEntries {
      worklogEntryId
      notes
      billable
      timespent
      afterHours
      startTime
      endTime
      technician
      ticket
    }
    listInfo { totalCount hasMore page pageSize }
  }
}`

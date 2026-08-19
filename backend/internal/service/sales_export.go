package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/repository"
	"backend/internal/trace"
	"backend/internal/xlsx"
)

var ErrInvalidExportRange = errors.New("invalid export range")

type SalesExportService interface {
	Build(ctx context.Context, from, to time.Time) (xlsx.SalesReport, error)
}

type salesExportService struct {
	orderRepo    repository.OrderRepository
	refundRepo   repository.RefundRepository
	productRepo  *repository.ProductRepository
	categoryRepo repository.CategoryRepository
}

func NewSalesExportService(
	orderRepo repository.OrderRepository,
	refundRepo repository.RefundRepository,
	productRepo *repository.ProductRepository,
	categoryRepo repository.CategoryRepository,
) SalesExportService {
	return &salesExportService{
		orderRepo:    orderRepo,
		refundRepo:   refundRepo,
		productRepo:  productRepo,
		categoryRepo: categoryRepo,
	}
}

var paymentMethodLabels = map[orderpayment.Method]string{
	orderpayment.MethodCASH:          "Bargeld",
	orderpayment.MethodCARD:          "Karte",
	orderpayment.MethodTWINT:         "TWINT",
	orderpayment.MethodGRATIS_GUEST:  "Gratis (Gast)",
	orderpayment.MethodGRATIS_VIP:    "Gratis (VIP)",
	orderpayment.MethodGRATIS_STAFF:  "Gratis (Mitarbeiter)",
	orderpayment.MethodGRATIS_PLUGIN: "Gratis (Aktion)",
}

// methodOrder keeps the cash-up table in a stable, cash-first order instead of
// whichever method happened to sell first.
var methodOrder = []orderpayment.Method{
	orderpayment.MethodCASH,
	orderpayment.MethodCARD,
	orderpayment.MethodTWINT,
	orderpayment.MethodGRATIS_GUEST,
	orderpayment.MethodGRATIS_VIP,
	orderpayment.MethodGRATIS_STAFF,
	orderpayment.MethodGRATIS_PLUGIN,
}

var orderStatusLabels = map[order.Status]string{
	order.StatusPaid:              "Bezahlt",
	order.StatusPartiallyRefunded: "Teilweise erstattet",
	order.StatusRefunded:          "Erstattet",
	order.StatusCancelled:         "Storniert",
	order.StatusPending:           "Offen",
}

const unknownMethodLabel = "Unbekannt"

func (s *salesExportService) Build(ctx context.Context, from, to time.Time) (xlsx.SalesReport, error) {
	ctx, finish := trace.StartSpan(ctx, "service", "sales.build_export")
	defer finish()

	if !to.After(from) {
		return xlsx.SalesReport{}, fmt.Errorf("%w: range end must be after start", ErrInvalidExportRange)
	}

	report := xlsx.SalesReport{From: from, To: to}

	orders, err := s.orderRepo.ListSettledInRange(ctx, from, to)
	if err != nil {
		return xlsx.SalesReport{}, err
	}
	trace.Data(ctx, "export.order_count", len(orders))
	if len(orders) == 0 {
		return report, nil
	}

	orderIDs := make([]string, len(orders))
	for i, o := range orders {
		orderIDs[i] = o.ID
	}
	refunds, err := s.refundRepo.ListSucceededByOrderIDs(ctx, orderIDs)
	if err != nil {
		return xlsx.SalesReport{}, err
	}

	catalog, err := s.loadCatalog(ctx)
	if err != nil {
		return xlsx.SalesReport{}, err
	}

	refundsByOrder := indexRefunds(refunds)
	agg := newSalesAggregator(catalog)
	for _, o := range orders {
		agg.addOrder(o, refundsByOrder[o.ID])
	}
	return agg.finish(report), nil
}

// salesCatalog is the product-side lookup the report needs: names, category paths
// and the list prices a bundle price is split by.
type salesCatalog struct {
	products   map[string]*ent.Product
	variants   map[string]*ent.ProductVariant
	categories map[string]*ent.Category
}

func (s *salesExportService) loadCatalog(ctx context.Context) (salesCatalog, error) {
	categories, err := s.categoryRepo.GetAll(ctx)
	if err != nil {
		return salesCatalog{}, err
	}
	products, err := s.productRepo.GetAll(ctx)
	if err != nil {
		return salesCatalog{}, err
	}

	c := salesCatalog{
		products:   make(map[string]*ent.Product, len(products)),
		variants:   make(map[string]*ent.ProductVariant),
		categories: make(map[string]*ent.Category, len(categories)),
	}
	for _, cat := range categories {
		c.categories[cat.ID] = cat
	}
	for _, p := range products {
		c.products[p.ID] = p
		for _, v := range p.Edges.Variants {
			c.variants[v.ID] = v
		}
	}
	return c, nil
}

// listPriceCents is what the article would have cost on its own — the weight a
// bundle price is distributed by.
func (c salesCatalog) listPriceCents(productID string, variantID *string) int64 {
	p, ok := c.products[productID]
	if !ok {
		return 0
	}
	price := p.PriceCents
	if variantID != nil {
		if v, ok := c.variants[*variantID]; ok {
			price += v.PriceAdjustmentCents
		}
	}
	return price
}

func (c salesCatalog) path(productID string) (category, subcategory string) {
	return categoryPath(c.products[productID], c.categories)
}

func (c salesCatalog) productName(line *ent.OrderLine) string {
	if p, ok := c.products[line.ProductID]; ok {
		return p.Name
	}
	// Falls back to the title snapshotted on the line, which survives renames.
	return line.Title
}

type orderRefunds struct {
	totalCents int64
	byLine     map[string]refundedLine
}

type refundedLine struct {
	quantity int
	cents    int64
}

func indexRefunds(refunds []*ent.Refund) map[string]orderRefunds {
	out := make(map[string]orderRefunds)
	for _, r := range refunds {
		entry, ok := out[r.OrderID]
		if !ok {
			entry = orderRefunds{byLine: make(map[string]refundedLine)}
		}
		entry.totalCents += r.AmountCents
		for _, rl := range r.Edges.RefundLines {
			line := entry.byLine[rl.OrderLineID]
			line.quantity += rl.Quantity
			line.cents += rl.AmountCents
			entry.byLine[rl.OrderLineID] = line
		}
		out[r.OrderID] = entry
	}
	return out
}

// saleRecord pairs a rendered row with the identity it aggregates under, so a
// renamed product still merges with its earlier sales.
type saleRecord struct {
	row       xlsx.SaleRow
	productID string
	variantID string
	free      bool
}

type articleKey struct {
	productID string
	variantID string
}

type salesAggregator struct {
	catalog  salesCatalog
	sales    []xlsx.SaleRow
	orders   []xlsx.OrderRow
	articles map[articleKey]*xlsx.ArticleRow
	methods  map[orderpayment.Method]*xlsx.MethodRow
	// unknown catches an order with no payment row, so the cash-up table still
	// adds up to the totals instead of quietly dropping it.
	unknown *xlsx.MethodRow
	totals  xlsx.SalesTotals
}

func newSalesAggregator(c salesCatalog) *salesAggregator {
	return &salesAggregator{
		catalog:  c,
		articles: make(map[articleKey]*xlsx.ArticleRow),
		methods:  make(map[orderpayment.Method]*xlsx.MethodRow),
	}
}

func (a *salesAggregator) addOrder(o *ent.Order, refunds orderRefunds) {
	method, hasMethod := primaryPaymentMethod(o)
	methodLabel := unknownMethodLabel
	if hasMethod {
		methodLabel = paymentMethodLabel(method)
	}
	// A gratis order still carries its full nominal total; it just never took any
	// money. Booking that as revenue would inflate the takings, so it is reported
	// at zero with the nominal value kept in its own column.
	gratis := hasMethod && isGratisMethod(method)

	records := a.flattenOrder(o, refunds, methodLabel, gratis)
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].row.Product != records[j].row.Product {
			return records[i].row.Product < records[j].row.Product
		}
		return records[i].row.Variant < records[j].row.Variant
	})

	itemCount := 0
	var linkedRefunds int64
	for _, rec := range records {
		itemCount += rec.row.Quantity
		linkedRefunds += rec.row.RefundedCents
		a.addArticle(rec)
		a.sales = append(a.sales, rec.row)
	}

	a.orders = append(a.orders, xlsx.OrderRow{
		At:            o.CreatedAt,
		OrderID:       o.ID,
		Channel:       channelLabel(o.Origin),
		Status:        orderStatusLabel(o.Status),
		PaymentMethod: methodLabel,
		ItemCount:     itemCount,
		TotalCents:    o.TotalCents,
		RefundedCents: refunds.totalCents,
		Contact:       contactOf(o),
	})

	a.totals.OrderCount++
	a.totals.ItemCount += itemCount
	if gratis {
		a.totals.GratisOrderCount++
		a.totals.GratisItemCount += itemCount
	}

	// Line refunds already ride on the rows; whatever a refund covered beyond them
	// — an arbitrary refund — is still money that left the till.
	unlinked := refunds.totalCents - linkedRefunds
	if unlinked < 0 || gratis {
		unlinked = 0
	}
	a.totals.UnlinkedRefunds += unlinked
	a.totals.RefundedCents += unlinked

	m := a.methodBucket(method, hasMethod)
	m.OrderCount++
	m.RefundedCents += unlinked
	for _, rec := range records {
		m.RevenueCents += rec.row.RevenueCents
		m.DepositCents += rec.row.DepositCents
		m.RefundedCents += rec.row.RefundedCents
		m.GratisCents += rec.row.GratisCents
	}
}

func (a *salesAggregator) methodBucket(method orderpayment.Method, known bool) *xlsx.MethodRow {
	if !known {
		if a.unknown == nil {
			a.unknown = &xlsx.MethodRow{Method: unknownMethodLabel}
		}
		return a.unknown
	}
	m, ok := a.methods[method]
	if !ok {
		m = &xlsx.MethodRow{Method: paymentMethodLabel(method)}
		a.methods[method] = m
	}
	return m
}

func (a *salesAggregator) addArticle(rec saleRecord) {
	key := articleKey{productID: rec.productID, variantID: rec.variantID}
	entry, ok := a.articles[key]
	if !ok {
		entry = &xlsx.ArticleRow{
			Category:    rec.row.Category,
			Subcategory: rec.row.Subcategory,
			Product:     rec.row.Product,
			Variant:     rec.row.Variant,
		}
		a.articles[key] = entry
	}
	entry.Quantity += rec.row.Quantity
	entry.RefundedQty += rec.row.RefundedQty
	if rec.row.FromBundle {
		entry.BundleQty += rec.row.Quantity
	}
	if rec.free {
		entry.GratisQty += rec.row.Quantity
	}
	entry.RevenueCents += rec.row.RevenueCents
	entry.DepositCents += rec.row.DepositCents
	entry.RefundedCents += rec.row.RefundedCents
	entry.GratisCents += rec.row.GratisCents

	a.totals.RevenueCents += rec.row.RevenueCents
	a.totals.DepositCents += rec.row.DepositCents
	a.totals.RefundedCents += rec.row.RefundedCents
	a.totals.GratisCents += rec.row.GratisCents
}

func (a *salesAggregator) finish(report xlsx.SalesReport) xlsx.SalesReport {
	articles := make([]xlsx.ArticleRow, 0, len(a.articles))
	for _, entry := range a.articles {
		articles = append(articles, *entry)
	}
	sort.SliceStable(articles, func(i, j int) bool {
		l, r := articles[i], articles[j]
		if l.Category != r.Category {
			return l.Category < r.Category
		}
		if l.Subcategory != r.Subcategory {
			return l.Subcategory < r.Subcategory
		}
		if l.Product != r.Product {
			return l.Product < r.Product
		}
		return l.Variant < r.Variant
	})

	methods := make([]xlsx.MethodRow, 0, len(a.methods)+1)
	for _, m := range methodOrder {
		if entry, ok := a.methods[m]; ok {
			methods = append(methods, *entry)
		}
	}
	if a.unknown != nil {
		methods = append(methods, *a.unknown)
	}

	report.Totals = a.totals
	report.Methods = methods
	report.Articles = articles
	report.Sales = a.sales
	report.Orders = a.orders
	return report
}

// flattenOrder turns an order's lines into one record per article actually handed
// over. A configured bundle is a grouping row that carries the price while its
// components sit at zero, so the price is split across the components and the
// parent never becomes a row of its own — otherwise article revenue would count
// the bundle twice.
func (a *salesAggregator) flattenOrder(o *ent.Order, refunds orderRefunds, methodLabel string, gratis bool) []saleRecord {
	children := make(map[string][]*ent.OrderLine)
	for _, l := range o.Edges.Lines {
		if l.ParentLineID != nil {
			children[*l.ParentLineID] = append(children[*l.ParentLineID], l)
		}
	}

	var records []saleRecord
	for _, line := range o.Edges.Lines {
		if line.ParentLineID != nil || line.LineType == orderline.LineTypeDeposit {
			continue
		}

		deposit, depositRefund := depositOf(children[line.ID], refunds)
		refunded := refunds.byLine[line.ID]
		gross := line.UnitPriceCents * int64(line.Quantity)

		components := componentsOf(children[line.ID])
		if line.LineType != orderline.LineTypeBundle || len(components) == 0 {
			records = append(records, a.record(o, line, methodLabel, gratis, saleAmounts{
				quantity:    line.Quantity,
				unitCents:   line.UnitPriceCents,
				revenue:     gross,
				deposit:     deposit,
				refunded:    refunded.cents + depositRefund,
				refundedQty: refunded.quantity,
			}, ""))
			continue
		}

		weights := make([]int64, len(components))
		for i, c := range components {
			weights[i] = a.catalog.listPriceCents(c.ProductID, c.VariantID) * int64(c.Quantity)
		}
		revenues := allocateCents(gross, weights)
		deposits := allocateCents(deposit, weights)
		refundShares := allocateCents(refunded.cents+depositRefund, weights)

		for i, c := range components {
			unit := int64(0)
			if c.Quantity > 0 {
				unit = revenues[i] / int64(c.Quantity)
			}
			records = append(records, a.record(o, c, methodLabel, gratis, saleAmounts{
				quantity:  c.Quantity,
				unitCents: unit,
				revenue:   revenues[i],
				deposit:   deposits[i],
				refunded:  refundShares[i],
				// One component is consumed per bundle unit, so a refunded bundle unit
				// takes back exactly one of each component.
				refundedQty: refunded.quantity,
				parentGross: gross,
			}, line.Title))
		}
	}
	return records
}

type saleAmounts struct {
	quantity    int
	unitCents   int64
	revenue     int64
	deposit     int64
	refunded    int64
	refundedQty int
	// parentGross is the bundle price a component was cut out of; a component of a
	// paid bundle is not a giveaway even when its own share rounds to zero.
	parentGross int64
}

func (a *salesAggregator) record(o *ent.Order, line *ent.OrderLine, methodLabel string, gratis bool, amounts saleAmounts, bundleTitle string) saleRecord {
	category, subcategory := a.catalog.path(line.ProductID)
	source := "Einzeln"
	if bundleTitle != "" {
		source = "Bundle: " + bundleTitle
	}

	row := xlsx.SaleRow{
		At:            o.CreatedAt,
		OrderID:       o.ID,
		Channel:       channelLabel(o.Origin),
		PaymentMethod: methodLabel,
		Category:      category,
		Subcategory:   subcategory,
		Product:       a.catalog.productName(line),
		Variant:       safeStr(line.VariantValue),
		Source:        source,
		FromBundle:    bundleTitle != "",
		Quantity:      amounts.quantity,
		UnitCents:     amounts.unitCents,
		RevenueCents:  amounts.revenue,
		DepositCents:  amounts.deposit,
		RefundedCents: amounts.refunded,
		RefundedQty:   amounts.refundedQty,
	}

	charged := amounts.revenue
	if bundleTitle != "" {
		charged = amounts.parentGross
	}
	if gratis {
		// Nominal value net of what was given back, so a cancelled gratis order
		// doesn't stay on the books as goods handed out.
		row.GratisCents = max(amounts.revenue-amounts.refunded, 0)
		row.RevenueCents = 0
		row.DepositCents = 0
		row.RefundedCents = 0
	}

	return saleRecord{
		row:       row,
		productID: line.ProductID,
		variantID: safeStr(line.VariantID),
		free:      gratis || charged == 0,
	}
}

func componentsOf(children []*ent.OrderLine) []*ent.OrderLine {
	var out []*ent.OrderLine
	for _, c := range children {
		if c.LineType == orderline.LineTypeComponent {
			out = append(out, c)
		}
	}
	return out
}

// depositOf returns what the line's deposit sibling charged and what has been
// returned of it. Deposits hang off the product line, never off a component.
func depositOf(children []*ent.OrderLine, refunds orderRefunds) (charged, refunded int64) {
	for _, c := range children {
		if c.LineType != orderline.LineTypeDeposit {
			continue
		}
		charged += c.UnitPriceCents * int64(c.Quantity)
		refunded += refunds.byLine[c.ID].cents
	}
	return charged, refunded
}

// allocateCents splits total along weights so the parts always add back up to
// total: the remainder goes to the largest fractions first.
func allocateCents(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	if len(weights) == 0 || total == 0 {
		return out
	}

	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		base := total / int64(len(weights))
		rest := total - base*int64(len(weights))
		for i := range out {
			out[i] = base
			if int64(i) < rest {
				out[i]++
			}
		}
		return out
	}

	type remainder struct {
		index int
		value int64
	}
	remainders := make([]remainder, len(weights))
	var assigned int64
	for i, w := range weights {
		share := total * w / sum
		out[i] = share
		assigned += share
		remainders[i] = remainder{index: i, value: total*w - share*sum}
	}
	sort.SliceStable(remainders, func(i, j int) bool { return remainders[i].value > remainders[j].value })
	for i := 0; assigned < total && i < len(remainders); i++ {
		out[remainders[i].index]++
		assigned++
	}
	return out
}

func primaryPaymentMethod(o *ent.Order) (orderpayment.Method, bool) {
	var earliest *ent.OrderPayment
	for _, p := range o.Edges.Payments {
		if earliest == nil || p.PaidAt.Before(earliest.PaidAt) {
			earliest = p
		}
	}
	if earliest == nil {
		return "", false
	}
	return earliest.Method, true
}

func isGratisMethod(method orderpayment.Method) bool {
	return strings.HasPrefix(string(method), "GRATIS_")
}

func paymentMethodLabel(method orderpayment.Method) string {
	if label, ok := paymentMethodLabels[method]; ok {
		return label
	}
	return string(method)
}

func orderStatusLabel(status order.Status) string {
	if label, ok := orderStatusLabels[status]; ok {
		return label
	}
	return string(status)
}

func channelLabel(origin order.Origin) string {
	if origin == order.OriginPos {
		return "POS"
	}
	return "Shop"
}

func contactOf(o *ent.Order) string {
	if o.ContactEmail != nil {
		return *o.ContactEmail
	}
	return ""
}

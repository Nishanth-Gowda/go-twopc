'use client';

import { Activity, Box, CircleCheck, Clock3, RefreshCw, Truck } from 'lucide-react';
import { FormEvent, useCallback, useEffect, useState } from 'react';

type Order = { id: string; sku: string; quantity: number; delivery_address: string; status: 'PREPARING' | 'COMMITTED' | 'ABORTED' };
type Inventory = { sku: string; available_quantity: number };
type Slot = { slot_id: number; order_id?: string; status: 'AVAILABLE' | 'PREPARED' | 'COMMITTED' };

const statusClass = { COMMITTED: 'status status-success', ABORTED: 'status status-danger', PREPARING: 'status status-warn', AVAILABLE: 'status status-neutral', PREPARED: 'status status-warn' };

export default function Home() {
  const [orders, setOrders] = useState<Order[]>([]);
  const [inventory, setInventory] = useState<Inventory[]>([]);
  const [slots, setSlots] = useState<Slot[]>([]);
  const [sku, setSKU] = useState('coffee-mug');
  const [quantity, setQuantity] = useState(1);
  const [address, setAddress] = useState('42 Main Street');
  const [delay, setDelay] = useState(false);
  const [result, setResult] = useState<{ id: string; status: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [lastUpdated, setLastUpdated] = useState('waiting for services');

  const refresh = useCallback(async () => {
    try {
      const [orderData, inventoryData, slotData] = await Promise.all([
        fetch('/api/orders').then((response) => response.json()),
        fetch('/api/store/state').then((response) => response.json()),
        fetch('/api/delivery/state').then((response) => response.json()),
      ]);
      setOrders(orderData);
      setInventory(inventoryData);
      setSlots(slotData);
      setLastUpdated(new Date().toLocaleTimeString());
    } catch {
      setLastUpdated('services unavailable');
    }
  }, []);

  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, 1000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const placeOrder = useCallback(async (input: { sku: string; quantity: number; delivery_address: string; simulateTimeout?: boolean }) => {
    setLoading(true);
    setResult(null);
    try {
      const response = await fetch('/api/orders', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(input.simulateTimeout ? { 'X-Delivery-Prepare-Delay-Ms': '3000' } : {}) },
        body: JSON.stringify(input),
      });
      const order = await response.json();
      setResult(order);
      await refresh();
      return order;
    } finally {
      setLoading(false);
    }
  }, [refresh]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await placeOrder({ sku, quantity, delivery_address: address, simulateTimeout: delay });
  }

  useEffect(() => {
    const modelContext = (document as unknown as { modelContext?: { registerTool: (tool: object, options: { signal: AbortSignal }) => void | Promise<void> } }).modelContext;
    if (!modelContext) return;
    const lifecycle = new AbortController();
    void Promise.resolve(modelContext.registerTool({
      name: 'place_order',
      title: 'Place order',
      description: 'Place an order through the visible 2PC workflow and return its final decision.',
      inputSchema: { type: 'object', properties: { sku: { type: 'string' }, quantity: { type: 'number' }, deliveryAddress: { type: 'string' }, simulateTimeout: { type: 'boolean' } }, required: ['sku', 'quantity', 'deliveryAddress'], additionalProperties: false },
      annotations: { readOnlyHint: false, untrustedContentHint: false },
      execute: async (input: unknown) => {
        if (!input || typeof input !== 'object') throw new Error('Order details are required');
        const value = input as { sku?: unknown; quantity?: unknown; deliveryAddress?: unknown; simulateTimeout?: unknown };
        if (typeof value.sku !== 'string' || typeof value.quantity !== 'number' || typeof value.deliveryAddress !== 'string') throw new Error('sku, quantity, and deliveryAddress are required');
        return placeOrder({ sku: value.sku, quantity: value.quantity, delivery_address: value.deliveryAddress, simulateTimeout: value.simulateTimeout === true });
      },
    }, { signal: lifecycle.signal })).catch(() => undefined);
    return () => lifecycle.abort();
  }, [placeOrder]);

  const committed = orders.filter((order) => order.status === 'COMMITTED').length;
  const aborted = orders.filter((order) => order.status === 'ABORTED').length;

  return <main>
    <header className="topbar"><div className="brand"><Activity size={20} /><span>2PC control room</span></div><div className="live"><span className="pulse" /> Live · refreshed {lastUpdated}</div></header>
    <section className="intro"><div><p className="eyebrow">ORDER SERVICE / COORDINATOR</p><h1>Watch one decision<br />move through three services.</h1></div><div className="protocol"><span>Order</span><i /> <span>Store</span><i /> <span>Delivery</span><small>prepare concurrently · persist decision · broadcast result</small></div></section>
    <section className="metrics"><Metric label="Orders observed" value={orders.length} icon={<Activity size={18} />} /><Metric label="Committed" value={committed} icon={<CircleCheck size={18} />} tone="green" /><Metric label="Aborted" value={aborted} icon={<Clock3 size={18} />} tone="red" /><Metric label="Available stock" value={inventory[0]?.available_quantity ?? '—'} icon={<Box size={18} />} tone="blue" /></section>
    <section className="workspace">
      <form className="order-form" onSubmit={submit}><div className="card-title"><span>Place an order</span><span className="step">01</span></div><label>SKU<input value={sku} onChange={(event) => setSKU(event.target.value)} /></label><label>Quantity<input type="number" min="1" value={quantity} onChange={(event) => setQuantity(Number(event.target.value))} /></label><label>Delivery address<input value={address} onChange={(event) => setAddress(event.target.value)} /></label><label className="toggle-row"><input type="checkbox" checked={delay} onChange={(event) => setDelay(event.target.checked)} /><span>Simulate delivery timeout</span><em>3s delay</em></label><button disabled={loading}>{loading ? 'Coordinating…' : 'Run 2PC transaction'} <span>→</span></button>{result && <div className={`result ${result.status === 'COMMITTED' ? 'result-success' : 'result-danger'}`}><strong>{result.status}</strong><span>{result.id}</span></div>}</form>
      <div className="service-panel"><div className="card-title"><span>Service state</span><button className="icon-button" onClick={refresh} aria-label="Refresh service state"><RefreshCw size={16} /></button></div><div className="service-row"><Box size={20} /><div><strong>StoreService</strong><small>{inventory[0]?.sku ?? 'coffee-mug'} inventory</small></div><b>{inventory[0]?.available_quantity ?? '—'}</b><span>available</span></div><div className="service-row"><Truck size={20} /><div><strong>DeliveryService</strong><small>reserved delivery slots</small></div><b>{slots.filter((slot) => slot.status === 'AVAILABLE').length}</b><span>free</span></div><div className="slot-grid">{slots.map((slot) => <div key={slot.slot_id} className={`slot ${slot.status === 'AVAILABLE' ? 'slot-free' : 'slot-used'}`}><small>slot {slot.slot_id}</small><strong>{slot.status === 'AVAILABLE' ? 'READY' : slot.status}</strong></div>)}</div></div>
    </section>
    <section className="orders-card"><div className="card-title"><span>Transaction ledger</span><span className="step">auto-refreshing</span></div><div className="table-head"><span>Order</span><span>Resource request</span><span>Destination</span><span>Decision</span></div>{orders.length === 0 ? <p className="empty">No orders yet. Start with the transaction form above.</p> : orders.map((order) => <div className="table-row" key={order.id}><code>{order.id.slice(0, 8)}</code><span>{order.quantity} × {order.sku}</span><span>{order.delivery_address}</span><span className={statusClass[order.status]}>{order.status}</span></div>)}</section>
  </main>;
}

function Metric({ label, value, icon, tone = 'plain' }: { label: string; value: string | number; icon: React.ReactNode; tone?: string }) {
  return <div className={`metric metric-${tone}`}><span>{icon}</span><div><strong>{value}</strong><small>{label}</small></div></div>;
}

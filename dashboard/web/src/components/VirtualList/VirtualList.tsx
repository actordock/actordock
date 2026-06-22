import { useEffect, useRef, useState, type ReactNode } from "react";
import "./VirtualList.css";

type VirtualListProps<T> = {
  items: T[];
  rowHeight: number;
  height: number;
  rowKey: (item: T, index: number) => string;
  renderRow: (item: T, index: number) => ReactNode;
  onScroll?: (scrollTop: number) => void;
  scrollToBottom?: boolean;
};

export function VirtualList<T>({
  items,
  rowHeight,
  height,
  rowKey,
  renderRow,
  onScroll,
  scrollToBottom = false,
}: VirtualListProps<T>) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  const totalHeight = items.length * rowHeight;
  const overscan = 4;
  const startIndex = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const endIndex = Math.min(
    items.length,
    Math.ceil((scrollTop + height) / rowHeight) + overscan,
  );
  const visibleItems = items.slice(startIndex, endIndex);

  useEffect(() => {
    if (!scrollToBottom || !viewportRef.current) {
      return;
    }
    viewportRef.current.scrollTop = totalHeight;
    setScrollTop(totalHeight);
  }, [scrollToBottom, totalHeight, items.length]);

  return (
    <div
      ref={viewportRef}
      className="virtual-list"
      style={{ height }}
      onScroll={(event) => {
        const top = event.currentTarget.scrollTop;
        setScrollTop(top);
        onScroll?.(top);
      }}
    >
      <div className="virtual-list__spacer" style={{ height: totalHeight }}>
        <div
          className="virtual-list__window"
          style={{ transform: `translateY(${startIndex * rowHeight}px)` }}
        >
          {visibleItems.map((item, offset) => {
            const index = startIndex + offset;
            return (
              <div
                key={rowKey(item, index)}
                className="virtual-list__row"
                style={{ height: rowHeight }}
              >
                {renderRow(item, index)}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

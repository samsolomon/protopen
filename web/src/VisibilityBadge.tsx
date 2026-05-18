import { Badge } from '@/components/ui/badge'

type VisibilityBadgeProps = {
  isPublic: boolean
}

export function VisibilityBadge({ isPublic }: VisibilityBadgeProps) {
  return (
    <Badge variant={isPublic ? 'secondary' : 'outline'}>
      {isPublic ? 'Public' : 'Private'}
    </Badge>
  )
}
